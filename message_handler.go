package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types/events"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// External server request structure
type ExternalServerRequest struct {
	Query       string `json:"query"`
	PhoneNumber string `json:"phone_number"`
}

// External server response structure
type ExternalServerResponse struct {
	Result      string `json:"result"`
	PhoneNumber string `json:"phone_number"`
	Error       string `json:"error,omitempty"`
}

// Handle incoming messages and forward to external server
func HandleIncomingMessage(msg *events.Message, logger waLog.Logger) {
	// Skip messages from ourselves to prevent infinite loops
	if msg.Info.IsFromMe {
		return
	}

	// Extract basic info
	chatJID := msg.Info.Chat.String()
	senderJID := msg.Info.Sender.String()
	content := extractTextContent(msg.Message)

	// Skip if no text content
	if strings.TrimSpace(content) == "" {
		logger.Debugf("Skipping message without text content from %s", senderJID)
		return
	}

	// Optional: Skip messages that look like bot responses to prevent loops
	if strings.Contains(strings.ToLower(content), "lo siento, no pude procesar") {
		logger.Debugf("Skipping potential bot response: %s", content)
		return
	}

	// Extract phone number from sender JID
	phoneNumber := extractPhoneFromJID(senderJID)
	if phoneNumber == "" {
		logger.Warnf("Could not extract phone number from JID: %s", senderJID)
		return
	}

	// Log incoming message to terminal
	logIncomingMessage(content, phoneNumber, logger)

	// Send to external server (asynchronous processing)
	go processMessageWithExternalServer(content, phoneNumber, chatJID, logger)
}

// Process message with external server and send response
func processMessageWithExternalServer(query, phoneNumber, chatJID string, logger waLog.Logger) {
	// Prepare request for external server
	request := ExternalServerRequest{
		Query:       query,
		PhoneNumber: phoneNumber,
	}

	// Send HTTP POST to external server
	response, err := sendToExternalServer(request, logger)
	if err != nil {
		logger.Errorf("Failed to get response from external server: %v", err)
		// Send error message back to user
		sendErrorResponse(chatJID, logger)
		return
	}

	// Send response back via WhatsApp
	if response.Result != "" {
		sendWhatsAppResponse(chatJID, response.Result, logger)
	} else {
		logger.Warnf("Empty response from external server for phone: %s", phoneNumber)
		sendErrorResponse(chatJID, logger)
	}
}

// Send request to external server
func sendToExternalServer(request ExternalServerRequest, logger waLog.Logger) (*ExternalServerResponse, error) {
	serverURL := os.Getenv("EXTERNAL_SERVER_URL")
	if serverURL == "" {
		return nil, fmt.Errorf("EXTERNAL_SERVER_URL not configured")
	}

	// Prepare JSON payload
	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP client with timeout
	timeoutStr := os.Getenv("EXTERNAL_SERVER_TIMEOUT")
	timeout := 120 * time.Second // default 2 minutos para consultas complejas
	if timeoutStr != "" {
		if t, err := strconv.Atoi(timeoutStr); err == nil {
			timeout = time.Duration(t) * time.Second
		}
	}

	client := &http.Client{
		Timeout: timeout,
	}

	logger.Infof("🔄 Sending request to external server for %s (timeout: %v)", request.PhoneNumber, timeout)

	// Send POST request
	resp, err := client.Post(serverURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	var response ExternalServerResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Check for HTTP error status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("external server returned status %d: %s", resp.StatusCode, response.Error)
	}

	logger.Infof("✅ External server response for %s: %s", request.PhoneNumber, response.Result)
	return &response, nil
}

// Send message via WhatsApp using existing function
func sendWhatsAppResponse(chatJID, message string, logger waLog.Logger) {
	// Send message using existing sendWhatsAppMessage function
	go func() {
		success, result := sendWhatsAppMessage(client, chatJID, message, "")
		if !success {
			logger.Errorf("Failed to send WhatsApp response: %s", result)
		} else {
			logger.Infof("✅ Response sent to %s: %s", chatJID, message)
		}
	}()
}

// Send error response to user
func sendErrorResponse(chatJID string, logger waLog.Logger) {
	errorMsg := "Lo siento, no pude procesar tu mensaje en este momento. Inténtalo más tarde."
	sendWhatsAppResponse(chatJID, errorMsg, logger)
}

// Extract text content from WhatsApp message (reuse from whatsapp-bridge)
func extractTextContent(message *waProto.Message) string {
	if message == nil {
		return ""
	}

	if message.Conversation != nil {
		return *message.Conversation
	}

	if message.ExtendedTextMessage != nil && message.ExtendedTextMessage.Text != nil {
		return *message.ExtendedTextMessage.Text
	}

	// Add other message types as needed
	return ""
}

// Extract phone number from JID
func extractPhoneFromJID(jid string) string {
	// JID format: "1234567890@s.whatsapp.net" or "1234567890@c.us"
	parts := strings.Split(jid, "@")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// Log incoming message to terminal
func logIncomingMessage(content, phoneNumber string, logger waLog.Logger) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("[%s] ← %s: %s\n", timestamp, phoneNumber, content)
}