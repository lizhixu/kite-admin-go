package sse

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type Client struct {
	UserID uint
	Chan   chan string
}

type Hub struct {
	mu         sync.RWMutex
	clients    map[uint][]*Client
	register   chan *Client
	unregister chan *Client
}

var defaultHub *Hub

func init() {
	defaultHub = &Hub{
		clients:    make(map[uint][]*Client),
		register:   make(chan *Client, 16),
		unregister: make(chan *Client, 16),
	}
	go defaultHub.run()
}

func Default() *Hub { return defaultHub }

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.UserID] = append(h.clients[client.UserID], client)
			h.mu.Unlock()
			log.Printf("SSE client connected: userID=%d", client.UserID)

		case client := <-h.unregister:
			h.mu.Lock()
			clients := h.clients[client.UserID]
			for i, c := range clients {
				if c == client {
					h.clients[client.UserID] = append(clients[:i], clients[i+1:]...)
					break
				}
			}
			if len(h.clients[client.UserID]) == 0 {
				delete(h.clients, client.UserID)
			}
			h.mu.Unlock()
			close(client.Chan)
			log.Printf("SSE client disconnected: userID=%d", client.UserID)
		}
	}
}

// Register registers a new SSE client for a user.
func (h *Hub) Register(userID uint) *Client {
	client := &Client{
		UserID: userID,
		Chan:   make(chan string, 8),
	}
	h.register <- client
	return client
}

// Unregister removes an SSE client.
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// NotifyUser sends an event to all SSE clients of a specific user.
func (h *Hub) NotifyUser(userID uint, data string) {
	h.mu.RLock()
	clients := h.clients[userID]
	h.mu.RUnlock()

	event := fmt.Sprintf("event: message\ndata: %s\n\n", data)
	for _, c := range clients {
		select {
		case c.Chan <- event:
		default:
			// client buffer full, skip
		}
	}
}

// NotifyUsers sends an event to multiple users.
func (h *Hub) NotifyUsers(userIDs []uint, data string) {
	for _, uid := range userIDs {
		h.NotifyUser(uid, data)
	}
}

// Broadcast sends an event to all connected clients.
func (h *Hub) Broadcast(data string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	event := fmt.Sprintf("event: message\ndata: %s\n\n", data)
	for _, clients := range h.clients {
		for _, c := range clients {
			select {
			case c.Chan <- event:
			default:
			}
		}
	}
}

// OnlineCount returns the number of connected users.
func (h *Hub) OnlineCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// FormatSSE formats a standard SSE keep-alive comment.
func FormatSSE() string {
	return fmt.Sprintf(": keepalive %d\n\n", time.Now().Unix())
}
