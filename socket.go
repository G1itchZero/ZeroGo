package main

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/Jeffail/gabs"
	log "github.com/Sirupsen/logrus"
	websocket "github.com/gorilla/websocket"
)

type UiSocket struct {
	WrapperKey   string
	Connection   *websocket.Conn
	Site         *Site
	Disconnected chan int
	MsgID        int
	sync.Mutex
}

func NewUiSocket(site *Site, wrapperKey string) *UiSocket {
	socket := UiSocket{
		WrapperKey:   wrapperKey,
		Disconnected: make(chan int),
		Site:         site,
		MsgID:        1,
	}
	return &socket
}

func (socket *UiSocket) Serve(ws *websocket.Conn) {
	socket.Connection = ws
	socket.Notification("done", "Hi from ZeroNet Golang client!")

	log.WithFields(log.Fields{
		"site":        socket.Site.Address,
		"wrapper_key": socket.WrapperKey,
	}).Info("New socket connection")
	go func() {
		for {
			select {
			case event := <-socket.Site.Downloader.OnChanges:
				log.WithFields(log.Fields{
					"event":       event,
					"wrapper_key": socket.WrapperKey,
				}).Debug("New socket event")
				info := socket.Site.GetInfo()
				info.Event = []interface{}{event.Type, event.Payload}
				socket.Cmd("setSiteInfo", info)
			}
		}
	}()
	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			log.WithFields(log.Fields{
				"site":        socket.Site.Address,
				"wrapper_key": socket.WrapperKey,
			}).Warn(err)
			return
		}
		message := Message{}
		err = json.Unmarshal(data, &message)
		if err != nil {
			continue
		}
		// log.WithFields(log.Fields{
		// 	"site":        socket.Site.Address,
		// 	"wrapper_key": socket.WrapperKey,
		// 	"massage":     message,
		// }).Info("Message")

		switch message.Cmd {
		case "fileQuery":
			go socket.fileQuery(message)
		case "siteDelete":
			go socket.siteDelete(message)
		case "siteInfo":
			go func(message Message) {
				// socket.Site.Wait()
				socket.Response(message.ID, socket.Site.GetInfo())
			}(message)
		case "siteList":
			go socket.siteList(message)
		case "serverInfo":
			go socket.Response(message.ID, GetServerInfo())
		case "feedQuery":
			go socket.feedQuery(message)
		}
	}
}

func (socket *UiSocket) siteDelete(message Message) {
	socket.Site.Manager.Remove(message.Params.(map[string]interface{})["address"].(string))
	socket.Notification("done", "Site deleted.")
}

func (socket *UiSocket) siteList(message Message) {
	sites := []SiteInfo{}
	infos, _ := socket.Site.Manager.GetSites().ChildrenMap()
	for _, site := range infos {
		sites = append(sites, site.Data().(SiteInfo))
	}
	socket.Response(message.ID, sites)
}

func (socket *UiSocket) fileQuery(message Message) {
	filename := message.Params.([]interface{})[0].(string)
	content, _ := socket.Site.GetFile(filename)
	if strings.HasSuffix(filename, ".json") {
		jsonContent, _ := gabs.ParseJSON(content)
		socket.Response(message.ID, []interface{}{jsonContent.Data()})
	} else {
		socket.Response(message.ID, []interface{}{string(content)})
	}
}

func (socket *UiSocket) feedQuery(message Message) {
	socket.Response(message.ID, []Post{
		{
			Body:      "@ZeroNet: Go, go, go!",
			Title:     "Project info",
			FeedName:  "Golang ZeroNet",
			Type:      "comment",
			DateAdded: int(time.Now().Unix()),
			URL:       "",
			Site:      socket.Site.Address,
		},
	})
}

func (socket *UiSocket) Notification(notificationType string, text string) {
	socket.Cmd("notification", []string{notificationType, text})
}

func (socket *UiSocket) Cmd(cmd string, params interface{}) {
	msg, _ := json.Marshal(Message{cmd, params, socket.MsgID})
	socket.Lock()
	socket.Connection.WriteMessage(websocket.TextMessage, msg)
	socket.MsgID++
	socket.Unlock()
}

func (socket *UiSocket) Response(to int, result interface{}) {
	msg, _ := json.Marshal(SocketResponse{"response", 1, to, result})
	socket.Lock()
	socket.Connection.WriteMessage(websocket.TextMessage, msg)
	socket.Unlock()
}

type Message struct {
	Cmd    string      `json:"cmd"`
	Params interface{} `json:"params"`
	ID     int         `json:"id"`
}

type SocketResponse struct {
	Cmd    string      `json:"cmd"`
	ID     int         `json:"id"`
	To     int         `json:"to"`
	Result interface{} `json:"result"`
}

func GetServerInfo() ServerInfo {
	return ServerInfo{
		IPExternal:     false,
		FileserverIP:   "*",
		Multiuser:      false,
		TorEnabled:     false,
		Plugins:        []string{},
		FileserverPort: 15441,
		MasterAddress:  "15Ni39HLKXmnXHRkuh8Cpj43AtDfTwc9Gv",
		Language:       "en",
		UIPort:         43111,
		Rev:            REV,
		UIIP:           "127.0.0.1",
		Platform:       "linux",
		Version:        VERSION,
		TorStatus:      "Not implemented",
		Debug:          false,
	}
}

type ServerInfo struct {
	IPExternal     bool     `json:"ip_external"`
	FileserverIP   string   `json:"fileserver_ip"`
	Multiuser      bool     `json:"multiuser"`
	TorEnabled     bool     `json:"tor_enabled"`
	Plugins        []string `json:"plugins"`
	FileserverPort int      `json:"fileserver_port"`
	MasterAddress  string   `json:"master_address"`
	Language       string   `json:"language"`
	UIPort         int      `json:"ui_port"`
	Rev            int      `json:"rev"`
	UIIP           string   `json:"ui_ip"`
	Platform       string   `json:"platform"`
	Version        string   `json:"version"`
	TorStatus      string   `json:"tor_status"`
	Debug          bool     `json:"debug"`
}

type Post struct {
	Body      string `json:"body"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Site      string `json:"site"`
	FeedName  string `json:"feed_name"`
	DateAdded int    `json:"date_added"`
	Type      string `json:"type"`
}
