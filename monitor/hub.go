package monitor

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

// Hub maintains the set of active clients and broadcasts messages to them
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan *WSMessage
	register   chan *Client
	unregister chan *Client
	clientCnt  atomic.Int64
	store      *Store
	mu         sync.RWMutex
}

// Client represents a WebSocket client connection
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

// NewHub creates a new Hub instance
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan *WSMessage, BroadcastChanSize),
		register:   make(chan *Client, RegisterChanSize),
		unregister: make(chan *Client, UnregisterChanSize),
	}
}

// SetStore wires the hub to the store so the hub can toggle realtime event work
// based on connected monitor websocket clients.
func (h *Hub) SetStore(store *Store) {
	h.mu.Lock()
	h.store = store
	h.mu.Unlock()
}

func (h *Hub) setRealtimeEnabled(enabled bool) {
	h.mu.RLock()
	store := h.store
	h.mu.RUnlock()
	if store == nil {
		return
	}
	store.SetRealtimeEnabled(enabled)
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			enabledRealtime := false
			if _, ok := h.clients[client]; !ok {
				h.clients[client] = true
				if h.clientCnt.Add(1) == 1 {
					enabledRealtime = true
				}
			}
			h.mu.Unlock()
			if enabledRealtime {
				h.setRealtimeEnabled(true)
			}

		case client := <-h.unregister:
			h.mu.Lock()
			disabledRealtime := false
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				if h.clientCnt.Add(-1) == 0 {
					disabledRealtime = true
				}
				close(client.send)
			}
			h.mu.Unlock()
			if disabledRealtime {
				h.setRealtimeEnabled(false)
			}

		case message := <-h.broadcast:
			if h.clientCnt.Load() == 0 {
				continue
			}

			data, err := json.Marshal(message)
			if err != nil {
				continue
			}

			var staleClients []*Client
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- data:
				default:
					staleClients = append(staleClients, client)
				}
			}
			h.mu.RUnlock()

			if len(staleClients) > 0 {
				h.mu.Lock()
				disabledRealtime := false
				for _, client := range staleClients {
					if _, ok := h.clients[client]; ok {
						delete(h.clients, client)
						if h.clientCnt.Add(-1) == 0 {
							disabledRealtime = true
						}
						close(client.send)
					}
				}
				h.mu.Unlock()
				if disabledRealtime {
					h.setRealtimeEnabled(false)
				}
			}
		}
	}
}

// Broadcast sends a message to all connected clients
func (h *Hub) Broadcast(msg *WSMessage) {
	if h.clientCnt.Load() == 0 {
		return
	}
	select {
	case h.broadcast <- msg:
	default:
		// Channel full, drop message
	}
}

// ClientCount returns the number of connected clients
func (h *Hub) ClientCount() int {
	return int(h.clientCnt.Load())
}

// ServeWs handles WebSocket requests from clients
func (h *Hub) ServeWs(c *gin.Context, store *Store) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := &Client{
		hub:  h,
		conn: conn,
		send: make(chan []byte, ClientSendChanSize),
	}

	select {
	case h.register <- client:
	default:
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "monitor hub busy"),
			time.Now().Add(WriteWait),
		)
		_ = conn.Close()
		return
	}

	// Send initial snapshot of all record summaries
	if store != nil {
		summaries := store.GetAllSummaries()
		snapshot := &WSMessage{
			Type:    WSMessageTypeSnapshot,
			Payload: summaries,
		}
		data, err := json.Marshal(snapshot)
		if err == nil {
			select {
			case client.send <- data:
			default:
			}
		}
	}

	// Start goroutines for reading and writing
	go client.writePump()
	go client.readPump()
}

// readPump pumps messages from the WebSocket connection to the hub
func (c *Client) readPump() {
	defer func() {
		select {
		case c.hub.unregister <- c:
		default:
			go func(h *Hub, client *Client) {
				h.unregister <- client
			}(c.hub, c)
		}
		c.conn.Close()
	}()

	c.conn.SetReadLimit(MaxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(PongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(PongWait))
		return nil
	})

	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			break
		}
		// We don't expect any messages from clients, just keep connection alive
	}
}

// writePump pumps messages from the hub to the WebSocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(PingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(WriteWait))
			if !ok {
				// Hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			if _, err := w.Write(message); err != nil {
				return
			}

			// Add queued messages to the same WebSocket message
			n := len(c.send)
			for i := 0; i < n; i++ {
				if _, err := w.Write([]byte{'\n'}); err != nil {
					break
				}
				if _, err := w.Write(<-c.send); err != nil {
					break
				}
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(WriteWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
