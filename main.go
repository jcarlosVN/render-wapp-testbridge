package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/skip2/go-qrcode"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

var (
	client    *whatsmeow.Client
	currentQR string
	needsAuth bool
	startTime time.Time
	mu        sync.RWMutex
)

// SendMessageRequest represents the request body for the send message API
type SendMessageRequest struct {
	Recipient string `json:"recipient"`
	Message   string `json:"message"`
	MediaPath string `json:"media_path,omitempty"`
}

// SendMessageResponse represents the response for the send message API
type SendMessageResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// Function to send a WhatsApp message
func sendWhatsAppMessage(client *whatsmeow.Client, recipient string, message string, mediaPath string) (bool, string) {
	if !client.IsConnected() {
		return false, "Not connected to WhatsApp"
	}

	// Create JID for recipient
	var recipientJID types.JID
	var err error

	// Check if recipient is a JID
	isJID := strings.Contains(recipient, "@")

	if isJID {
		// Parse the JID string
		recipientJID, err = types.ParseJID(recipient)
		if err != nil {
			return false, fmt.Sprintf("Error parsing JID: %v", err)
		}
	} else {
		// Create JID from phone number
		recipientJID = types.JID{
			User:   recipient,
			Server: "s.whatsapp.net", // For personal chats
		}
	}

	msg := &waProto.Message{}

	// Check if we have media to send
	if mediaPath != "" {
		// Read media file
		mediaData, err := os.ReadFile(mediaPath)
		if err != nil {
			return false, fmt.Sprintf("Error reading media file: %v", err)
		}

		// Determine media type and mime type based on file extension
		fileExt := strings.ToLower(mediaPath[strings.LastIndex(mediaPath, ".")+1:])
		var mediaType whatsmeow.MediaType
		var mimeType string

		// Handle different media types
		switch fileExt {
		// Image types
		case "jpg", "jpeg":
			mediaType = whatsmeow.MediaImage
			mimeType = "image/jpeg"
		case "png":
			mediaType = whatsmeow.MediaImage
			mimeType = "image/png"
		case "gif":
			mediaType = whatsmeow.MediaImage
			mimeType = "image/gif"
		case "webp":
			mediaType = whatsmeow.MediaImage
			mimeType = "image/webp"

		// Audio types
		case "ogg":
			mediaType = whatsmeow.MediaAudio
			mimeType = "audio/ogg; codecs=opus"

		// Video types
		case "mp4":
			mediaType = whatsmeow.MediaVideo
			mimeType = "video/mp4"
		case "avi":
			mediaType = whatsmeow.MediaVideo
			mimeType = "video/avi"
		case "mov":
			mediaType = whatsmeow.MediaVideo
			mimeType = "video/quicktime"

		// Document types (for any other file type)
		default:
			mediaType = whatsmeow.MediaDocument
			mimeType = "application/octet-stream"
		}

		// Upload media to WhatsApp servers
		resp, err := client.Upload(context.Background(), mediaData, mediaType)
		if err != nil {
			return false, fmt.Sprintf("Error uploading media: %v", err)
		}

		// Create the appropriate message type based on media type
		switch mediaType {
		case whatsmeow.MediaImage:
			msg.ImageMessage = &waProto.ImageMessage{
				Caption:       proto.String(message),
				Mimetype:      proto.String(mimeType),
				URL:           &resp.URL,
				DirectPath:    &resp.DirectPath,
				MediaKey:      resp.MediaKey,
				FileEncSHA256: resp.FileEncSHA256,
				FileSHA256:    resp.FileSHA256,
				FileLength:    &resp.FileLength,
			}
		case whatsmeow.MediaAudio:
			// Handle ogg audio files
			var seconds uint32 = 30 // Default fallback
			var waveform []byte = nil

			// Try to analyze the ogg file
			if strings.Contains(mimeType, "ogg") {
				seconds = uint32(len(mediaData) / 8000) // Simple estimation
				if seconds < 1 {
					seconds = 1
				} else if seconds > 300 {
					seconds = 300
				}
				waveform = generateSimpleWaveform(seconds)
			}

			msg.AudioMessage = &waProto.AudioMessage{
				Mimetype:      proto.String(mimeType),
				URL:           &resp.URL,
				DirectPath:    &resp.DirectPath,
				MediaKey:      resp.MediaKey,
				FileEncSHA256: resp.FileEncSHA256,
				FileSHA256:    resp.FileSHA256,
				FileLength:    &resp.FileLength,
				Seconds:       proto.Uint32(seconds),
				PTT:           proto.Bool(true),
				Waveform:      waveform,
			}
		case whatsmeow.MediaVideo:
			msg.VideoMessage = &waProto.VideoMessage{
				Caption:       proto.String(message),
				Mimetype:      proto.String(mimeType),
				URL:           &resp.URL,
				DirectPath:    &resp.DirectPath,
				MediaKey:      resp.MediaKey,
				FileEncSHA256: resp.FileEncSHA256,
				FileSHA256:    resp.FileSHA256,
				FileLength:    &resp.FileLength,
			}
		case whatsmeow.MediaDocument:
			msg.DocumentMessage = &waProto.DocumentMessage{
				Title:         proto.String(mediaPath[strings.LastIndex(mediaPath, "/")+1:]),
				Caption:       proto.String(message),
				Mimetype:      proto.String(mimeType),
				URL:           &resp.URL,
				DirectPath:    &resp.DirectPath,
				MediaKey:      resp.MediaKey,
				FileEncSHA256: resp.FileEncSHA256,
				FileSHA256:    resp.FileSHA256,
				FileLength:    &resp.FileLength,
			}
		}
	} else {
		msg.Conversation = proto.String(message)
	}

	// Send message
	_, err = client.SendMessage(context.Background(), recipientJID, msg)

	if err != nil {
		return false, fmt.Sprintf("Error sending message: %v", err)
	}

	return true, fmt.Sprintf("Message sent to %s", recipient)
}

// Clean corrupted database if foreign key errors occur
func cleanDatabase(dbPath string, logger waLog.Logger) error {
	logger.Warnf("🧹 Cleaning potentially corrupted database...")
	
	// Remove the database file
	if err := os.RemoveAll("store"); err != nil {
		return fmt.Errorf("failed to remove store directory: %v", err)
	}
	
	// Recreate the directory
	if err := os.MkdirAll("store", 0755); err != nil {
		return fmt.Errorf("failed to recreate store directory: %v", err)
	}
	
	logger.Infof("✅ Database cleaned successfully")
	return nil
}

// Recreate WhatsApp client without restarting the entire service
func recreateClient(logger waLog.Logger) error {
	logger.Infof("🔄 Recreating WhatsApp client with clean database...")
	
	// Disconnect current client if exists
	if client != nil {
		client.Disconnect()
		client = nil
	}
	
	// Clean database
	if err := cleanDatabase("store", logger); err != nil {
		return fmt.Errorf("failed to clean database: %v", err)
	}
	
	// Create new database connection
	dbLog := waLog.Stdout("Database", "INFO", true)
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:store/whatsapp.db?_foreign_keys=on", dbLog)
	if err != nil {
		return fmt.Errorf("failed to create new database connection: %v", err)
	}
	
	// Create new device store
	deviceStore := container.NewDevice()
	logger.Infof("Created new device store")
	
	// Create new client
	client = whatsmeow.NewClient(deviceStore, logger)
	if client == nil {
		return fmt.Errorf("failed to create new WhatsApp client")
	}
	
	// Add event handlers to new client
	client.AddEventHandler(func(evt interface{}) {
		switch evt.(type) {
		case *events.Connected:
			mu.Lock()
			needsAuth = false
			currentQR = ""
			mu.Unlock()
			logger.Infof("✅ Connected to WhatsApp")
		case *events.LoggedOut:
			mu.Lock()
			needsAuth = true
			currentQR = ""
			mu.Unlock()
			logger.Warnf("🔴 Device logged out, need to restart for new QR")
			go func() {
				time.Sleep(2 * time.Second)
				if err := recreateClient(logger); err != nil {
					logger.Errorf("Failed to recreate client after logout: %v", err)
				}
			}()
		}
	})
	
	// Start authentication process
	mu.Lock()
	needsAuth = true
	currentQR = ""
	mu.Unlock()
	
	logger.Infof("🔐 Starting authentication for new client...")
	qrChan, _ := client.GetQRChannel(context.Background())
	err = client.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect new client: %v", err)
	}
	
	// Handle QR codes for new client
	go func() {
		for evt := range qrChan {
			if evt.Event == "code" {
				mu.Lock()
				currentQR = evt.Code
				needsAuth = true
				mu.Unlock()
				logger.Infof("📱 New QR Code available at /api/qr")
			} else if evt.Event == "success" {
				mu.Lock()
				needsAuth = false
				currentQR = ""
				mu.Unlock()
				logger.Infof("✅ QR Authentication successful!")
				break
			}
		}
	}()
	
	logger.Infof("✅ Client recreated successfully")
	return nil
}

// Generate a simple waveform for voice messages
func generateSimpleWaveform(duration uint32) []byte {
	const waveformLength = 64
	waveform := make([]byte, waveformLength)

	rand.Seed(int64(duration))
	baseAmplitude := 35.0

	for i := range waveform {
		pos := float64(i) / float64(waveformLength)
		val := baseAmplitude * math.Sin(pos*math.Pi*8)
		val += (rand.Float64() - 0.5) * 15
		val += 50

		if val < 0 {
			val = 0
		} else if val > 100 {
			val = 100
		}

		waveform[i] = byte(val)
	}

	return waveform
}

// Generate QR as base64 data URL using native Go QR library
func generateQRDataURL(qrString string) string {
	// Generate QR code PNG using go-qrcode library
	// This handles the WhatsApp binary data properly
	png, err := qrcode.Encode(qrString, qrcode.Medium, 300)
	if err != nil {
		// Fallback to text if QR generation fails
		return ""
	}
	
	// Convert to base64 data URL for HTML display
	encoded := base64.StdEncoding.EncodeToString(png)
	return fmt.Sprintf("data:image/png;base64,%s", encoded)
}


// Start REST API server with all endpoints
func startRESTServer(port string) {
	// Health check endpoint
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `
		<html>
		<head>
			<title>WhatsApp Render Bridge</title>
			<style>
				body { font-family: Arial, sans-serif; text-align: center; padding: 20px; background: #f5f5f5; }
				.container { max-width: 600px; margin: 0 auto; background: white; padding: 30px; border-radius: 10px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
				.status { padding: 10px; border-radius: 5px; margin: 10px 0; }
				.connected { background: #d4edda; color: #155724; }
				.disconnected { background: #f8d7da; color: #721c24; }
				.pending { background: #fff3cd; color: #856404; }
				a { color: #007bff; text-decoration: none; margin: 0 10px; }
				a:hover { text-decoration: underline; }
			</style>
		</head>
		<body>
			<div class="container">
				<h1>🚀 WhatsApp Render Bridge</h1>
				<p>API REST para envío de mensajes WhatsApp</p>
				<div class="status %s">
					<strong>Estado:</strong> %s
				</div>
				<p>
					<a href="/api/status">📊 Status JSON</a>
					<a href="/api/qr">📱 QR Code</a>
				</p>
				<hr>
				<h3>📋 Endpoints disponibles:</h3>
				<p><strong>POST /api/send</strong> - Enviar mensajes</p>
				<p><strong>GET /api/qr</strong> - Ver código QR</p>
				<p><strong>GET /api/status</strong> - Estado del servicio</p>
				<hr>
				<button onclick="cleanDatabase()" style="background: #fd7e14; color: white; border: none; padding: 10px 20px; border-radius: 5px; cursor: pointer; margin: 5px;">
					🧹 Limpiar base de datos
				</button>
				<script>
					function cleanDatabase() {
						if (confirm('¿Estás seguro? Esto eliminará la sesión actual y requerirá una nueva autenticación QR.')) {
							fetch('/api/clean', {method: 'POST'})
							.then(response => response.json())
							.then(data => {
								alert(data.message + ' Redirigiendo al QR...');
								if (data.success) {
									setTimeout(() => window.location.href = '/api/qr', 2000);
								}
							})
							.catch(error => alert('Error: ' + error));
						}
					}
				</script>
			</div>
		</body>
		</html>`,
		getStatusClass(),
		getStatusText())
	})

	// QR Code endpoint - Browser display
	http.HandleFunc("/api/qr", func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		qr := currentQR
		needsAuthStatus := needsAuth
		mu.RUnlock()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		if qr != "" && needsAuthStatus {
			qrDataURL := generateQRDataURL(qr)
			if qrDataURL == "" {
				// Fallback if QR generation fails
				fmt.Fprintf(w, `
				<html><body style="text-align: center; padding: 20px; font-family: Arial;">
					<h2>❌ Error generando QR</h2>
					<p>Refresca la página para intentar de nuevo</p>
					<script>setTimeout(() => location.reload(), 3000);</script>
				</body></html>`)
				return
			}
			fmt.Fprintf(w, `
			<html>
			<head>
				<title>WhatsApp QR Code</title>
				<meta charset="UTF-8">
				<meta name="viewport" content="width=device-width, initial-scale=1">
				<style>
					body { font-family: Arial, sans-serif; text-align: center; padding: 10px; background: #f5f5f5; }
					.container { max-width: 400px; margin: 0 auto; background: white; padding: 20px; border-radius: 10px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
					.qr-image { margin: 20px 0; padding: 15px; background: white; border: 2px solid #25D366; border-radius: 10px; }
					.qr-image img { max-width: 100%%; height: auto; border-radius: 5px; }
					.status { color: #dc3545; font-weight: bold; margin: 15px 0; }
					.instructions { background: #e7f3ff; padding: 15px; border-radius: 5px; margin: 15px 0; text-align: left; }
					.refresh { color: #28a745; font-weight: bold; }
					.whatsapp-color { color: #25D366; }
					@media (max-width: 480px) {
						.container { padding: 15px; margin: 10px; }
						.qr-image { margin: 15px 0; padding: 10px; }
					}
				</style>
				<script>
					setTimeout(() => {
						location.reload();
					}, 5000);
				</script>
			</head>
			<body>
				<div class="container">
					<h2><span class="whatsapp-color">📱 Escanea con WhatsApp móvil</span></h2>
					<div class="status">🔴 Desconectado - Necesita autenticación</div>
					<div class="qr-image">
						<img src="%s" alt="QR Code para WhatsApp Web" />
					</div>
					<div class="instructions">
						<strong>📋 Instrucciones:</strong><br>
						1. Abre WhatsApp en tu teléfono<br>
						2. Toca Menú ⋮ > WhatsApp Web<br>
						3. Escanea este código QR<br>
						4. ¡Listo! Podrás enviar mensajes
					</div>
					<div class="refresh">🔄 Auto-refresh en 5 segundos...</div>
					<p><a href="/">← Volver al inicio</a></p>
				</div>
			</body>
			</html>`, qrDataURL)
		} else if client != nil && client.IsConnected() {
			fmt.Fprintf(w, `
			<html>
			<head>
				<title>WhatsApp Status</title>
				<meta name="viewport" content="width=device-width, initial-scale=1">
				<style>
					body { font-family: Arial, sans-serif; text-align: center; padding: 20px; background: #f5f5f5; }
					.container { max-width: 400px; margin: 0 auto; background: white; padding: 30px; border-radius: 10px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
					.status { color: #28a745; font-weight: bold; font-size: 18px; margin: 20px 0; }
					.uptime { background: #e7f3ff; padding: 15px; border-radius: 5px; margin: 15px 0; }
				</style>
			</head>
			<body>
				<div class="container">
					<h2>✅ WhatsApp Conectado</h2>
					<div class="status">🟢 Servicio activo y funcionando</div>
					<div class="uptime">
						<strong>⏱️ Uptime:</strong> %s
					</div>
					<p>El bridge está listo para enviar mensajes.</p>
					<p>
						<a href="/api/status">📊 Ver status detallado</a><br>
						<a href="/">← Volver al inicio</a>
					</p>
				</div>
			</body>
			</html>`, time.Since(startTime).Round(time.Second))
		} else {
			fmt.Fprintf(w, `
			<html>
			<head>
				<title>WhatsApp Status</title>
				<meta name="viewport" content="width=device-width, initial-scale=1">
				<style>
					body { font-family: Arial, sans-serif; text-align: center; padding: 20px; background: #f5f5f5; }
					.container { max-width: 400px; margin: 0 auto; background: white; padding: 30px; border-radius: 10px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
					.status { color: #ffc107; font-weight: bold; font-size: 16px; margin: 20px 0; }
				</style>
				<script>
					setTimeout(() => {
						location.reload();
					}, 3000);
				</script>
			</head>
			<body>
				<div class="container">
					<h2>⏳ Iniciando conexión...</h2>
					<div class="status">🟡 Estableciendo conexión con WhatsApp...</div>
					<p>Por favor espera unos segundos.</p>
					<p>🔄 Auto-refresh en 3 segundos...</p>
					<p><a href="/">← Volver al inicio</a></p>
				</div>
			</body>
			</html>`)
		}
	})

	// Status endpoint - JSON API
	http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		qr := currentQR
		needsAuthStatus := needsAuth
		mu.RUnlock()

		status := map[string]interface{}{
			"connected":    client != nil && client.IsConnected(),
			"needs_qr":     needsAuthStatus,
			"has_qr":       qr != "",
			"uptime":       time.Since(startTime).String(),
			"qr_url":       fmt.Sprintf("https://%s/api/qr", r.Host),
			"service":      "whatsapp-render-bridge",
			"version":      "1.0.0",
			"timestamp":    time.Now().Unix(),
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	// Clean database endpoint (for fixing corruption)
	http.HandleFunc("/api/clean", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		
		logger := waLog.Stdout("Clean", "INFO", true)
		logger.Warnf("🧹 Manual database cleanup requested")
		
		// Recreate client without service restart
		go func() {
			if err := recreateClient(logger); err != nil {
				logger.Errorf("Failed to recreate client: %v", err)
			}
		}()
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Database cleaned successfully. New QR code will be available shortly at /api/qr",
		})
	})


	// Send message endpoint
	http.HandleFunc("/api/send", func(w http.ResponseWriter, r *http.Request) {
		// Only allow POST requests
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse the request body
		var req SendMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request format", http.StatusBadRequest)
			return
		}

		// Validate request
		if req.Recipient == "" {
			http.Error(w, "Recipient is required", http.StatusBadRequest)
			return
		}

		if req.Message == "" && req.MediaPath == "" {
			http.Error(w, "Message or media path is required", http.StatusBadRequest)
			return
		}

		fmt.Printf("📤 Send request: %s -> %s\n", req.Recipient, req.Message)

		// Send the message
		success, message := sendWhatsAppMessage(client, req.Recipient, req.Message, req.MediaPath)
		fmt.Printf("📨 Result: %v - %s\n", success, message)

		// Set response headers
		w.Header().Set("Content-Type", "application/json")

		// Set appropriate status code
		if !success {
			w.WriteHeader(http.StatusInternalServerError)
		}

		// Send response
		json.NewEncoder(w).Encode(SendMessageResponse{
			Success: success,
			Message: message,
		})
	})

	// Start the server
	fmt.Printf("🚀 Starting WhatsApp Bridge on port %s\n", port)
	fmt.Printf("🌐 Access: http://localhost:%s\n", port)
	
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Printf("❌ Server error: %v\n", err)
	}
}

// Helper functions for status display
func getStatusClass() string {
	mu.RLock()
	needsAuthStatus := needsAuth
	mu.RUnlock()
	
	// First check if we need authentication
	if needsAuthStatus {
		return "disconnected"
	}
	
	// Then check if client exists and is actually connected
	if client != nil && client.IsConnected() {
		return "connected"
	}
	
	return "pending"
}

func getStatusText() string {
	mu.RLock()
	needsAuthStatus := needsAuth
	mu.RUnlock()
	
	// First check if we need authentication
	if needsAuthStatus {
		return "🔴 Necesita autenticación QR"
	}
	
	// Then check if client exists and is actually connected
	if client != nil && client.IsConnected() {
		return "🟢 Conectado y funcionando"
	}
	
	return "🟡 Iniciando conexión..."
}

func main() {
	startTime = time.Now()
	
	// Initialize needsAuth to true on startup
	mu.Lock()
	needsAuth = true
	mu.Unlock()
	
	// Set up logger
	logger := waLog.Stdout("Client", "INFO", true)
	logger.Infof("🚀 Starting WhatsApp Render Bridge...")

	// Get port from environment (Render provides this)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Create database connection for storing session data only
	dbLog := waLog.Stdout("Database", "INFO", true)

	// Create directory for database if it doesn't exist
	if err := os.MkdirAll("store", 0755); err != nil {
		logger.Errorf("Failed to create store directory: %v", err)
		return
	}

	// Try to connect to database
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:store/whatsapp.db?_foreign_keys=on", dbLog)
	if err != nil {
		logger.Errorf("Failed to connect to database: %v", err)
		
		// If connection fails, try cleaning the database
		logger.Warnf("Attempting to clean potentially corrupted database...")
		if cleanErr := cleanDatabase("store", logger); cleanErr != nil {
			logger.Errorf("Failed to clean database: %v", cleanErr)
			return
		}
		
		// Retry connection after cleaning
		container, err = sqlstore.New(context.Background(), "sqlite3", "file:store/whatsapp.db?_foreign_keys=on", dbLog)
		if err != nil {
			logger.Errorf("Failed to connect to database after cleaning: %v", err)
			return
		}
	}

	// Get device store - This contains session information
	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		if err == sql.ErrNoRows {
			// No device exists, create one
			deviceStore = container.NewDevice()
			logger.Infof("Created new device")
		} else if strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
			// Database is corrupted, clean and retry
			logger.Warnf("Database corruption detected (FOREIGN KEY constraint), cleaning...")
			if cleanErr := cleanDatabase("store", logger); cleanErr != nil {
				logger.Errorf("Failed to clean corrupted database: %v", cleanErr)
				return
			}
			
			// Reconnect to cleaned database
			container, err = sqlstore.New(context.Background(), "sqlite3", "file:store/whatsapp.db?_foreign_keys=on", dbLog)
			if err != nil {
				logger.Errorf("Failed to reconnect to cleaned database: %v", err)
				return
			}
			
			// Create new device after cleanup
			deviceStore = container.NewDevice()
			logger.Infof("Created new device after database cleanup")
		} else {
			logger.Errorf("Failed to get device: %v", err)
			return
		}
	}

	// Create client instance
	client = whatsmeow.NewClient(deviceStore, logger)
	if client == nil {
		logger.Errorf("Failed to create WhatsApp client")
		return
	}

	// Event handling for connection and QR
	client.AddEventHandler(func(evt interface{}) {
		switch evt.(type) {
		case *events.Connected:
			mu.Lock()
			needsAuth = false
			currentQR = ""
			mu.Unlock()
			logger.Infof("✅ Connected to WhatsApp")
		case *events.LoggedOut:
			mu.Lock()
			needsAuth = true
			currentQR = ""
			mu.Unlock()
			logger.Warnf("🔴 Device logged out, need to restart for new QR")
			// Trigger a new QR generation by restarting the auth process
			go func() {
				time.Sleep(2 * time.Second)
				if err := recreateClient(logger); err != nil {
					logger.Errorf("Failed to recreate client after logout: %v", err)
				}
			}()
		}
	})

	// Start REST API server in background
	go startRESTServer(port)

	// Connect to WhatsApp
	if client.Store.ID == nil {
		// No ID stored, need to authenticate
		mu.Lock()
		needsAuth = true
		mu.Unlock()
		
		logger.Infof("🔐 No session found, starting authentication...")
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			logger.Errorf("Failed to connect: %v", err)
			return
		}

		// Handle QR codes
		go func() {
			for evt := range qrChan {
				if evt.Event == "code" {
					mu.Lock()
					currentQR = evt.Code
					needsAuth = true
					mu.Unlock()
					logger.Infof("📱 QR Code available at /api/qr")
				} else if evt.Event == "success" {
					mu.Lock()
					needsAuth = false
					currentQR = ""
					mu.Unlock()
					logger.Infof("✅ QR Authentication successful!")
					break
				}
			}
		}()
	} else {
		// Already logged in, just connect
		logger.Infof("📱 Existing session found, connecting...")
		err = client.Connect()
		if err != nil {
			logger.Errorf("Failed to connect: %v", err)
			return
		}
		
		// Wait a moment to check connection status
		time.Sleep(3 * time.Second)
		
		// If not connected after existing session, may need new auth
		if !client.IsConnected() {
			logger.Infof("🔄 Existing session failed, starting fresh authentication...")
			mu.Lock()
			needsAuth = true
			mu.Unlock()
			if err := recreateClient(logger); err != nil {
				logger.Errorf("Failed to recreate client: %v", err)
			}
		}
	}

	logger.Infof("🌐 WhatsApp Bridge ready on port %s", port)

	// Keep the main goroutine alive
	exitChan := make(chan os.Signal, 1)
	signal.Notify(exitChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for termination signal
	<-exitChan

	fmt.Println("👋 Shutting down...")
	if client != nil {
		client.Disconnect()
	}
}