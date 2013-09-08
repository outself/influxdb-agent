package main

import (
	"code.google.com/p/goprotobuf/proto"
	log "code.google.com/p/log4go"
	"github.com/errplane/errplane-go-common/agent"
	"github.com/garyburd/go-websocket/websocket"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"
	. "utils"
)

type WebsocketClient struct {
	ws         *websocket.Conn
	send       chan *agent.Response
	pingPeriod time.Duration
}

func NewWebsocketClient() *WebsocketClient {
	cl := &WebsocketClient{send: make(chan *agent.Response), pingPeriod: (AgentConfig.WebsocketPing * 9) / 10}
	return cl
}

func (self *WebsocketClient) writePump() {
	ticker := time.NewTicker(self.pingPeriod)
	defer func() {
		ticker.Stop()
	}()

	for {
		select {
		case res := <-self.send:
			if data, err := proto.Marshal(res); err == nil {
				if err := self.ws.WriteMessage(websocket.OpBinary, data); err != nil {
					log.Error("Couldn't write response to Anomalous", err)
				}
			} else {
				log.Error("Couldn't marshal the reponse to send to Anomalous", err, res)
			}
		case <-ticker.C:
			if self.ws == nil {
				log.Warn("Not connected to Anomalous")
			} else {
				if err := self.ws.WriteMessage(websocket.OpPing, []byte{}); err != nil {
					log.Error("Erorr writing ping: ", err)
				}
			}
		}
	}
}

func (self *WebsocketClient) readPump() {
	for {
		if self.ws == nil {
			time.Sleep(1 * time.Second)
			self.connect()
		} else {
			self.ws.SetReadDeadline(time.Now().Add(AgentConfig.WebsocketPing))
			op, r, err := self.ws.NextReader()
			if err != nil {
				log.Error("Error reading from websocket: ", err)
				time.Sleep(100 * time.Millisecond)
				self.connect()
			}
			if op == websocket.OpBinary {
				data, err := ioutil.ReadAll(r)
				if err != nil {
					log.Error("Error reading binary from websocket:", err)
				} else {
					request := &agent.Request{}
					proto.Unmarshal(data, request)

					// TODO: actually process the request
					t := agent.Response_METRICS
					r := &agent.Response{Type: &t}
					r.TimeSeries = make([]*agent.TimeSeries, 1, 1)
					seriesName := "foobar"
					r.TimeSeries[0] = &agent.TimeSeries{Name: &seriesName}
					self.send <- r
				}
			} else if op == websocket.OpPong {
				self.ws.SetReadDeadline(time.Now().Add(AgentConfig.WebsocketPing))
			}
		}
	}
}

func (self *WebsocketClient) connect() error {
	if self.ws != nil {
		self.ws.Close()
	}
	c, err := net.Dial("tcp", AgentConfig.ConfigWebsocket)
	if err != nil {
		log.Error("Dial: %v", err)
		return err
	}
	u, _ := url.Parse("/channel?database=" + AgentConfig.AppKey + AgentConfig.Environment + "&host=" + AgentConfig.Hostname + "&api_key=" + AgentConfig.ApiKey)
	ws, _, err := websocket.NewClient(c, u, http.Header{}, 1024, 1024)
	if err != nil {
		log.Error("NewClient: %v", err)
		return err
	}
	self.ws = ws
	if self.ws != nil {
		t := agent.Response_IDENTIFICATION
		db := AgentConfig.Database()
		res := &agent.Response{Type: &t, AgentName: &AgentConfig.Hostname, Database: &db}
		if data, err := proto.Marshal(res); err == nil {
			if err := self.ws.WriteMessage(websocket.OpBinary, data); err != nil {
				log.Error("Couldn't write Identification to Anomalous", err)
			}
		} else {
			log.Error("Couldn't marshal the reponse to send to Anomalous", err, res)
		}
	}
	return nil
}
