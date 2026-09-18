package browser

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Protocol errors are deliberately not forwarded: Chrome may echo captured data.
var errCDP = errors.New("browser connection unavailable or command rejected")

type cdpMessage struct {
	ID      int             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Session string          `json:"sessionId,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

type cdpClient struct {
	connection *websocket.Conn
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	next       int
	pending    map[int]chan cdpMessage
	events     chan cdpMessage
	done       chan struct{}
	overflow   bool
	readError  error
}

func connectCDP(ctx context.Context, endpoint string) (*cdpClient, error) {
	connection, response, err := websocket.Dial(ctx, endpoint, nil)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		return nil, errCDP
	}
	connection.SetReadLimit(768 << 10)
	lifetime, cancel := context.WithCancel(ctx)
	c := &cdpClient{connection: connection, ctx: lifetime, cancel: cancel, pending: map[int]chan cdpMessage{}, events: make(chan cdpMessage, 128), done: make(chan struct{})}
	go c.read()
	return c, nil
}

func (c *cdpClient) read() {
	defer close(c.done)
	defer close(c.events)
	defer c.cancel()
	defer c.connection.CloseNow()
	for {
		kind, data, err := c.connection.Read(c.ctx)
		if err != nil || kind != websocket.MessageText {
			c.mu.Lock()
			c.readError = err
			c.mu.Unlock()
			return
		}
		var message cdpMessage
		if json.Unmarshal(data, &message) != nil {
			c.mu.Lock()
			c.readError = errors.New("malformed protocol message")
			c.mu.Unlock()
			return
		}
		if message.ID != 0 {
			c.mu.Lock()
			response := c.pending[message.ID]
			delete(c.pending, message.ID)
			c.mu.Unlock()
			if response != nil {
				response <- message
			}
			continue
		}
		select {
		case c.events <- message:
		case <-c.ctx.Done():
			return
		default:
			c.mu.Lock()
			c.overflow = true
			c.mu.Unlock()
			return
		}
	}
}

func (c *cdpClient) call(ctx context.Context, session, method string, params any, result any) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	parameters, err := json.Marshal(params)
	if err != nil {
		return errCDP
	}
	c.mu.Lock()
	c.next++
	id := c.next
	response := make(chan cdpMessage, 1)
	c.pending[id] = response
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, id); c.mu.Unlock() }()
	data, _ := json.Marshal(cdpMessage{ID: id, Session: session, Method: method, Params: parameters})
	if c.connection.Write(ctx, websocket.MessageText, data) != nil {
		return errCDP
	}
	select {
	case reply := <-response:
		if len(reply.Error) != 0 {
			return errCDP
		}
		if result != nil && json.Unmarshal(reply.Result, result) != nil {
			return errCDP
		}
		return nil
	case <-ctx.Done():
		return errCDP
	case <-c.done:
		return errCDP
	}
}

func (c *cdpClient) close() { c.cancel(); _ = c.connection.CloseNow(); <-c.done }
