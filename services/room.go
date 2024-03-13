package services

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/topfreegames/pitaya/v2"
	"github.com/topfreegames/pitaya/v2/logger"
	"time"

	"github.com/google/uuid"
	"github.com/topfreegames/pitaya/v2/component"
	"github.com/topfreegames/pitaya/v2/examples/demo/protos"
	"github.com/topfreegames/pitaya/v2/timer"
)

type (
	// Room represents a component that contains a bundle of room related handler
	// like Join/Message
	Room struct {
		component.Base
		timer    *timer.Timer
		app      pitaya.Pitaya
		Stats    *protos.Stats
		MoveChan chan MoveReq
	}

	// UserMessage represents a message that user sent
	UserMessage struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}

	MoveReq struct {
		X float32 `json:"X"`
		Y float32 `json:"Y"`
		Z float32 `json:"Z"`
	}

	// Stats exports the room status
	Stats struct {
		outboundBytes int
		inboundBytes  int
	}

	// RPCResponse represents a rpc message
	RPCResponse struct {
		Msg string `json:"msg"`
	}

	// SendRPCMsg represents a rpc message
	SendRPCMsg struct {
		ServerID string `json:"serverId"`
		Route    string `json:"route"`
		Msg      string `json:"msg"`
	}

	// NewUser message will be received when new user join room
	NewUser struct {
		Content string `json:"content"`
	}

	// AllMembers contains all members uid
	AllMembers struct {
		Members []string `json:"members"`
	}

	// JoinResponse represents the result of joining room
	JoinResponse struct {
		Code   int    `json:"code"`
		Result string `json:"result"`
	}
)

// NewRoom returns a new room
func NewRoom(app pitaya.Pitaya) *Room {
	return &Room{
		app:      app,
		Stats:    &protos.Stats{},
		MoveChan: make(chan MoveReq, 100),
	}
}

// Init runs on service initialization
func (r *Room) Init() {
	r.app.GroupCreate(context.Background(), "room")
}

// AfterInit component lifetime callback
func (r *Room) AfterInit() {
	logger.Log.Debug("[DEBU] Room AfterInit")

	r.timer = pitaya.NewTimer(time.Minute, func() {
		count, err := r.app.GroupCountMembers(context.Background(), "room")
		println("UserCount: Time=>", time.Now().String(), "Count=>", count, "Error=>", err)
		println("OutboundBytes", r.Stats.OutboundBytes)
		println("InboundBytes", r.Stats.OutboundBytes)
	})

	ticker := time.NewTicker(time.Second)

	go func() {
		for {
			select {
			case t := <-ticker.C:
				logger.Log.Debug("[DEBU] step cur time: %v \n", t)
				for req := range r.MoveChan {
					logger.Log.Debug("[DEBU] step move req: %v \n", req)
					ctx := context.Background()
					err := r.app.GroupBroadcast(ctx, "connector", "room", "onMove", req)
					if err != nil {
						logger.Log.Debug("Error broadcasting message")
						logger.Log.Debug(err)
					}
				}
			}
		}
	}()
}

// Entry is the entrypoint
func (r *Room) Entry(ctx context.Context, msg []byte) (*protos.JoinResponse, error) {
	logger := pitaya.GetDefaultLoggerFromCtx(ctx) // The default logger contains a requestId, the route being executed and the sessionId
	s := r.app.GetSessionFromCtx(ctx)

	err := s.Bind(ctx, uuid.New().String())
	if err != nil {
		logger.Error("Failed to bind session")
		logger.Error(err)
		return nil, pitaya.Error(err, "RH-000", map[string]string{"failed": "bind"})
	}
	return &protos.JoinResponse{Result: "ok"}, nil
}

// GetSessionData gets the session data
func (r *Room) GetSessionData(ctx context.Context) (*SessionData, error) {
	s := r.app.GetSessionFromCtx(ctx)
	return &SessionData{
		Data: s.GetData(),
	}, nil
}

// SetSessionData sets the session data
func (r *Room) SetSessionData(ctx context.Context, data *SessionData) ([]byte, error) {
	logger := pitaya.GetDefaultLoggerFromCtx(ctx)
	s := r.app.GetSessionFromCtx(ctx)
	err := s.SetData(data.Data)
	if err != nil {
		logger.Error("Failed to set session data")
		logger.Error(err)
		return nil, err
	}
	err = s.PushToFront(ctx)
	if err != nil {
		return nil, err
	}
	return []byte("success"), nil
}

// Notify push is a notify route that triggers a push to a session
func (r *Room) NotifyPush(ctx context.Context) {
	s := r.app.GetSessionFromCtx(ctx)
	r.app.SendPushToUsers("testPush", &protos.RPCMsg{Msg: "test"}, []string{s.UID()}, "connector")
}

// Join room
func (r *Room) Join(ctx context.Context) (*protos.JoinResponse, error) {
	logger := pitaya.GetDefaultLoggerFromCtx(ctx)
	s := r.app.GetSessionFromCtx(ctx)
	err := r.app.GroupAddMember(ctx, "room", s.UID())
	if err != nil {
		logger.Error("Failed to join room")
		logger.Error(err)
		return nil, err
	}
	members, err := r.app.GroupMembers(ctx, "room")
	if err != nil {
		logger.Error("Failed to get members")
		logger.Error(err)
		return nil, err
	}
	s.Push("onMembers", &protos.AllMembers{Members: members})
	err = r.app.GroupBroadcast(ctx, "connector", "room", "onNewUser", &protos.NewUser{Content: fmt.Sprintf("New user: %d", s.ID())})
	if err != nil {
		logger.Error("Failed to broadcast onNewUser")
		logger.Error(err)
		return nil, err
	}
	return &protos.JoinResponse{Result: "success"}, nil
}

func (r *Room) Move(ctx context.Context, buf []byte) {
	logger := pitaya.GetDefaultLoggerFromCtx(ctx)

	var msg MoveReq

	json.Unmarshal(buf, &msg)

	r.MoveChan <- msg

	logger.Debugf("move req: %v", msg)
}

// Leave room
//func (r *Room) Leave(ctx context.Context) ([]byte, error) {
//	logger := pitaya.GetDefaultLoggerFromCtx(ctx)
//	s := r.app.GetSessionFromCtx(ctx)
//	err := r.app.GroupRemoveMember(ctx, "room", s.UID())
//	if err != nil {
//		logger.Error(err)
//		return []byte("failed"), err
//	}
//	return []byte("success"), err
//}

// Message sync last message to all members
func (r *Room) Message(ctx context.Context, msg *protos.UserMessage) {
	logger := pitaya.GetDefaultLoggerFromCtx(ctx)
	err := r.app.GroupBroadcast(ctx, "connector", "room", "onMessage", msg)
	if err != nil {
		logger.Error("Error broadcasting message")
		logger.Error(err)
	}
}

// SendRPC sends rpc
func (r *Room) SendRPC(ctx context.Context, msg *protos.SendRPCMsg) (*protos.RPCRes, error) {
	logger := pitaya.GetDefaultLoggerFromCtx(ctx)
	ret := &protos.RPCRes{}
	err := r.app.RPCTo(ctx, msg.ServerId, msg.Route, ret, &protos.RPCMsg{Msg: msg.Msg})
	if err != nil {
		logger.Errorf("Failed to execute RPCTo %s - %s", msg.ServerId, msg.Route)
		logger.Error(err)
		return nil, pitaya.Error(err, "RPC-000")
	}
	return ret, nil
}

// MessageRemote just echoes the given message
func (r *Room) MessageRemote(ctx context.Context, msg *protos.UserMessage, b bool, s string) (*protos.UserMessage, error) {
	return msg, nil
}
