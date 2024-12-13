package ui

import (
	"context"

	"github.com/gorilla/websocket"
)

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients.
	broadcast chan []byte

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client

	// Logger
	//Logger *zerolog.Logger

	ctx     context.Context
	Execute func(args string)
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte, 1),
		register:   make(chan *Client, 1),
		unregister: make(chan *Client, 1),
		clients:    make(map[*Client]bool, 1),
	}
}

func (h *Hub) Run(ctx context.Context, cancel context.CancelFunc) {
	h.ctx = ctx
	// main hub loop. Wait for client events, register, unregister or broadcast
	// ends on last client unregistering or ctx
hubrun:
	for {
		select {
		case <-ctx.Done():
			break hubrun
		case client := <-h.register:
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
				// last client, we terminate
				if len(h.clients) == 0 {
					cancel()
					break hubrun
				}
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.clients, client)
				}
			}
		}
	}
}

func (b *Hub) Subscribe(conn *websocket.Conn) *Client {
	client := &Client{Hub: b, Conn: conn, Send: make(chan []byte, 256)}
	b.register <- client
	return client
}

func (b *Hub) Unsubscribe(client *Client) {
	b.unregister <- client
}
func (h *Hub) SendMessage(msg []byte) {
	h.broadcast <- msg
}

// implements io.Writer, used for logging
func (h *Hub) Write(msg []byte) (int, error) {
	h.SendMessage(msg)
	return len(msg), nil
}
