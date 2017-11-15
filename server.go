package main

import (
	"fmt"
	"net/http"
	"path/filepath"
	"sync"

	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/olahol/melody"
)

type gameSession struct {
	p [2]*melody.Session
	c string
}

type gameMessage struct {
	T    string `json:"type"`
	Data []byte `json:"data"`
	Msg  string `json:"msg"`
}

func msgToBytes(t, msg string) []byte {
	s := gameMessage{t, []byte(msg), ""}
	b, err := json.Marshal(s)
	if err != nil {
		return []byte("")
	}
	return b
}

func main() {
	r := gin.New()
	m := melody.New()

	size := 65536
	m.Upgrader = &websocket.Upgrader{
		ReadBufferSize:  size,
		WriteBufferSize: size,
	}
	m.Config.MaxMessageSize = int64(size)
	m.Config.MessageBufferSize = 2048

	r.Static("/jsnes", "./jsnes")

	r.GET("/", func(c *gin.Context) {
		http.ServeFile(c.Writer, c.Request, "index.html")
	})

	r.GET("/gamelist", func(c *gin.Context) {
		files, _ := filepath.Glob("*.nes")
		c.JSON(200, gin.H{"games": files})
	})

	r.GET("/games?name=:name", func(c *gin.Context) {
		name := c.Params.ByName("name")
		http.ServeFile(c.Writer, c.Request, name)
	})

	r.GET("/ws", func(c *gin.Context) {
		m.HandleRequest(c.Writer, c.Request)
	})

	var mutex sync.Mutex
	partners := make(map[string]*gameSession)
	pool := make(map[*melody.Session]string)

	m.HandleConnect(func(s *melody.Session) {
		mutex.Lock()
		pool[s] = ""
		mutex.Unlock()
	})

	m.HandleMessage(func(s *melody.Session, msg []byte) {
		var ms gameMessage
		json.Unmarshal(msg, &ms)
		fmt.Printf("Type: %s, Data: %s\n", ms.T, ms.Data)
		mutex.Lock()
		p := partners[pool[s]]
		mutex.Unlock()
		if ms.T == "connect" {
			mutex.Lock()
			pool[s] = ms.Msg
			mutex.Unlock()
			if p != nil {
				var pn, mn int
				if p.p[0] == s {
					mn, pn = 1, 2
				} else {
					mn, pn = 2, 1
				}
				mutex.Lock()
				partners[ms.Msg].c = ms.Msg
				mutex.Unlock()
				s.Write(msgToBytes("join", string(mn)))
				s.Write(msgToBytes("join", string(pn)))
			} else {
				mutex.Lock()
				partners[string(ms.Data)] = &gameSession{[2]*melody.Session{s, nil}, string(ms.Data)}
				mutex.Unlock()
			}
		} else {
			if p.p[0] == s {
				p.p[1].Write(msg)
			} else {
				p.p[0].Write(msg)
			}
		}
	})

	m.HandleDisconnect(func(s *melody.Session) {
		var pn int8
		mutex.Lock()
		if pool[s] == "" {
			mutex.Unlock()
			return
		}
		p := partners[pool[s]]
		mutex.Unlock()
		if p.p[0] == s {
			pn = 1
		} else {
			pn = 0
		}
		if p.p[pn] != nil {
			p.p[pn].Write([]byte("part"))
		} else {
			mutex.Lock()
			delete(partners, p.c)
			mutex.Unlock()
		}
	})

	r.Run(":5000")
}
