package main

import (
	"code.google.com/p/goprotobuf/proto"
	log "code.google.com/p/log4go"
	"datastore"
	"github.com/errplane/errplane-go-common/agent"
	"github.com/garyburd/go-websocket/websocket"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"utils"
)

type WebsocketClient struct {
	ws                  *websocket.Conn
	send                chan *agent.Response
	pingPeriod          time.Duration
	config              *utils.Config
	anomaliesDetector   *AnomaliesDetector
	timeSeriesDatastore *datastore.TimeseriesDatastore
	snapshotDatastore   *datastore.SnapshotDatastore
}

func NewWebsocketClient(config *utils.Config, anomaliesDetector *AnomaliesDetector, timeSeriesDatastore *datastore.TimeseriesDatastore, snapshotDatastore *datastore.SnapshotDatastore) *WebsocketClient {
	cl := &WebsocketClient{
		send:                make(chan *agent.Response),
		config:              config,
		pingPeriod:          (config.WebsocketPing * 9) / 10,
		anomaliesDetector:   anomaliesDetector,
		timeSeriesDatastore: timeSeriesDatastore,
		snapshotDatastore:   snapshotDatastore,
	}
	return cl
}

func (self *WebsocketClient) Start() {
	self.connect()
	go self.writePump()
	go self.readPump()
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
			self.ws.SetReadDeadline(time.Now().Add(self.pingPeriod))
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
					self.handleRequest(request)
				}
			} else if op == websocket.OpPong {
				self.ws.SetReadDeadline(time.Now().Add(self.pingPeriod))
			}
		}
	}
}

func (self *WebsocketClient) handleRequest(request *agent.Request) {
	switch *request.Type {
	case agent.Request_CONFIG_RELOAD:
		self.anomaliesDetector.ForceMonitorConfigUpdate()
	case agent.Request_METRICS:
		self.send <- self.readMetrics(request)
	case agent.Request_SNAPSHOT:
		self.send <- self.getSnapshot(request)
	default:
		log.Error("Don't know how to handle request: ", request)

		// TODO: actually process the request
		t := agent.Response_METRICS
		r := &agent.Response{Type: &t}
		r.TimeSeries = make([]*agent.TimeSeries, 1, 1)
		seriesName := "foobar"
		r.TimeSeries[0] = &agent.TimeSeries{Name: &seriesName}
		self.send <- r
	}
}

func (self *WebsocketClient) getSnapshot(request *agent.Request) *agent.Response {
	snapshot, _ := self.snapshotDatastore.GetSnapshot(request.GetSnapshotId())
	t := agent.Response_SNAPSHOT
	r := &agent.Response{Type: &t}
	r.Snapshot = snapshot
	return r
}

func (self *WebsocketClient) readMetrics(request *agent.Request) *agent.Response {
	t := agent.Response_METRICS
	r := &agent.Response{Type: &t}
	r.TimeSeries = make([]*agent.TimeSeries, 0)
	defaultLimit := int64(1)
	params := &datastore.GetParams{StartTime: int64(1), EndTime: time.Now().Unix(), Database: self.config.Database(), IncludeContext: true}
	if request.StartTime != nil {
		// since they set a start time the default limit should be much higher
		defaultLimit = int64(1000)
		params.StartTime = *request.StartTime
	}
	if request.EndTime != nil {
		params.EndTime = *request.EndTime
	}
	if request.Limit != nil {
		defaultLimit = *request.Limit
	}

	metrics := map[string]bool{}
	for _, n := range request.MetricNames {
		metrics[n] = true
	}

	rawStatPrefix := self.config.Hostname + "."
	// look up the regex matches
	if len(request.MetricRegexes) > 0 {
		addNamesToLookup := func(metricName string) {
			for _, regex := range request.MetricRegexes {
				matches, err := regexp.MatchString(regex, metricName)
				if err != nil {
					log.Error("Error while using regex %s. Error: %s", regex, err)
					continue
				}
				if matches {
					if strings.HasPrefix(metricName, rawStatPrefix) {
						log.Info("metric: %s - %s - %s", rawStatPrefix, metricName, strings.TrimPrefix(metricName, rawStatPrefix))
						metrics[strings.TrimPrefix(metricName, rawStatPrefix)] = true
						break
					}
				}
			}
		}
		st := time.Now().Add(-(1 * time.Hour)).Unix()
		if request.StartTime != nil {
			st = *request.StartTime
		}
		self.timeSeriesDatastore.ReadSeriesIndex(params.Database, 1000, st, addNamesToLookup)
	}

	// now look up the metrics
	for n, _ := range metrics {
		params.Limit = defaultLimit
		params.TimeSeries = rawStatPrefix + n
		name := n
		ts := &agent.TimeSeries{Name: &name, Points: make([]*agent.Point, 0)}
		addPoint := func(point *agent.Point) error {
			ts.Points = append(ts.Points, point)
			return nil
		}
		self.timeSeriesDatastore.ReadSeries(params, addPoint)
		r.TimeSeries = append(r.TimeSeries, ts)
	}

	return r
}

func (self *WebsocketClient) connect() error {
	if self.ws != nil {
		self.ws.Close()
	}
	c, err := net.Dial("tcp", self.config.ConfigWebsocket)
	if err != nil {
		log.Error("Dial: %v", err)
		return err
	}
	u, _ := url.Parse("/channel?database=" + self.config.AppKey + self.config.Environment + "&host=" + self.config.Hostname + "&api_key=" + self.config.ApiKey)
	ws, _, err := websocket.NewClient(c, u, http.Header{}, 1024, 1024)
	if err != nil {
		log.Error("NewClient: %v", err)
		return err
	}
	self.ws = ws
	if self.ws != nil {
		t := agent.Response_IDENTIFICATION
		db := self.config.Database()
		res := &agent.Response{Type: &t, AgentName: &self.config.Hostname, Database: &db}
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
