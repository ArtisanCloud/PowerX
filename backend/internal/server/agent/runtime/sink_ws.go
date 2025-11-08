// internal/server/agent/runtime/sink_ws.go
package runtime

import (
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gorilla/websocket"
)

type WSSink struct{ conn *websocket.Conn }

func NewWSSink(conn *websocket.Conn) *WSSink { return &WSSink{conn: conn} }

func (w *WSSink) Emit(event string, payload any) error {
	env, _ := dto.NewEnv(event, payload)
	return w.conn.WriteJSON(env)
}
