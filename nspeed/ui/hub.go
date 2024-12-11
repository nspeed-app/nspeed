package ui

import "context"

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients.
	Broadcast chan []byte

	// Register requests from the clients.
	Register chan *Client

	// Unregister requests from clients.
	Unregister chan *Client

	ctx     context.Context
	Execute func(args string)
}

func NewHub() *Hub {
	return &Hub{
		Broadcast:  make(chan []byte, 100),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) Run(ctx context.Context, cancel context.CancelFunc) {
	h.ctx = ctx
hubrun:
	for {
		select {
		case <-ctx.Done():
			break hubrun
		case client := <-h.Register:
			h.clients[client] = true
		case client := <-h.Unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
				// last client, we terminate
				if len(h.clients) == 0 {
					cancel()
					break hubrun
				}
			}
		case message := <-h.Broadcast:
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
