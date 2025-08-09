package main

import (
	"context"
	"database/sql"
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

// Generate QR as HTML image or link
func generateQRHTML(qrString string) string {
	// Generate QR code as image URL
	return fmt.Sprintf(`
		<div class="qr-container">
			<img src="https://api.qrserver.com/v1/create-qr-code/?size=200x200&data=%s" 
				 alt="QR Code" 
				 style="border: 1px solid #ddd; padding: 10px; background: white;">
			<br>
			<small style="margin-top: 10px; display: block; color: #666;">
				Si el QR no carga, usa este texto:<br>
				<code style="font-size: 10px; word-break: break-all;">%s</code>
			</small>
		</div>
	`, qrString, qrString)
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
			fmt.Fprintf(w, `
			<html>
			<head>
				<title>WhatsApp QR Code</title>
				<meta name="viewport" content="width=device-width, initial-scale=1">
				<style>
					body { font-family: Arial, sans-serif; text-align: center; padding: 10px; background: #f5f5f5; }
					.container { max-width: 400px; margin: 0 auto; background: white; padding: 20px; border-radius: 10px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
					.qr { font-family: monospace; font-size: 6px; line-height: 6px; margin: 20px 0; background: white; padding: 10px; border: 1px solid #ddd; }
					.status { color: #dc3545; font-weight: bold; margin: 15px 0; }
					.instructions { background: #e7f3ff; padding: 15px; border-radius: 5px; margin: 15px 0; }
					.refresh { color: #28a745; }
				</style>
				<script>
					setTimeout(() => {
						location.reload();
					}, 5000);
				</script>
			</head>
			<body>
				<div class="container">
					<h2>📱 Escanea con WhatsApp móvil</h2>
					<div class="status">🔴 Desconectado - Necesita autenticación</div>
					<div class="qr">%s</div>
					<div class="instructions">
						<strong>📋 Instrucciones:</strong><br>
						1. Abre WhatsApp en tu teléfono<br>
						2. Toca Menú ⋮ > WhatsApp Web<br>
						3. Escanea este código QR
					</div>
					<div class="refresh">🔄 Auto-refresh en 5 segundos...</div>
					<p><a href="/">← Volver al inicio</a></p>
				</div>
			</body>
			</html>`, generateQRHTML(qr))
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
	if client != nil && client.IsConnected() {
		return "connected"
	}
	mu.RLock()
	needsAuthStatus := needsAuth
	mu.RUnlock()
	if needsAuthStatus {
		return "disconnected"
	}
	return "pending"
}

func getStatusText() string {
	if client != nil && client.IsConnected() {
		return "🟢 Conectado y funcionando"
	}
	mu.RLock()
	needsAuthStatus := needsAuth
	mu.RUnlock()
	if needsAuthStatus {
		return "🔴 Necesita autenticación QR"
	}
	return "🟡 Iniciando conexión..."
}

func main() {
	startTime = time.Now()
	
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

	container, err := sqlstore.New(context.Background(), "sqlite3", "file:store/whatsapp.db?_foreign_keys=on", dbLog)
	if err != nil {
		logger.Errorf("Failed to connect to database: %v", err)
		return
	}

	// Get device store - This contains session information
	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		if err == sql.ErrNoRows {
			// No device exists, create one
			deviceStore = container.NewDevice()
			logger.Infof("Created new device")
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
			mu.Unlock()
			logger.Warnf("🔴 Device logged out, QR scan needed")
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
