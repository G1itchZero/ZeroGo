package main

import (
	"encoding/json"
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	websocket "github.com/gorilla/websocket"
)

type UiSocket struct {
	WrapperKey   string
	Connection   *websocket.Conn
	Site         *Site
	Disconnected chan int
}

func NewUiSocket(site *Site, wrapperKey string) *UiSocket {
	fmt.Println("new socket", wrapperKey)
	socket := UiSocket{
		WrapperKey:   wrapperKey,
		Disconnected: make(chan int),
		Site:         site,
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
	for {
		// Read
		_, data, err := ws.ReadMessage()
		if err != nil {
			// log.Fatal(err)
			log.WithFields(log.Fields{
				"site":        socket.Site.Address,
				"wrapper_key": socket.WrapperKey,
			}).Warn(err)
			return
		}
		message := Message{}
		json.Unmarshal(data, &message)
		log.WithFields(log.Fields{
			"site":        socket.Site.Address,
			"wrapper_key": socket.WrapperKey,
			"massage":     message,
		}).Info("Message")

		switch message.Cmd {
		case "siteInfo":
			socket.Site.Wait()
			socket.Response(message.ID, socket.Site.GetInfo())
		case "siteList":
			sites := []SiteInfo{}
			infos, _ := socket.Site.Manager.GetSites().ChildrenMap()
			for _, site := range infos {
				sites = append(sites, site.Data().(SiteInfo))
			}
			socket.Response(message.ID, sites)
		case "serverInfo":
			socket.Response(message.ID, GetServerInfo())
		case "feedQuery":
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
	}

	// c.OnMessage(func(data []byte) {
	//
	// })
}

func (socket *UiSocket) Notification(notificationType string, text string) {
	socket.Cmd("notification", []string{notificationType, text})
}

func (socket *UiSocket) Cmd(cmd string, params interface{}) {
	msg, _ := json.Marshal(Message{cmd, params, 1})
	socket.Connection.WriteMessage(websocket.TextMessage, msg)
}

func (socket *UiSocket) Response(to int, result interface{}) {
	msg, _ := json.Marshal(SocketResponse{"response", 1, to, result})
	socket.Connection.WriteMessage(websocket.TextMessage, msg)
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
