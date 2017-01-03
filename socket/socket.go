package socket

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/G1itchZero/ZeroGo/site"
	"github.com/G1itchZero/ZeroGo/site_manager"
	"github.com/G1itchZero/ZeroGo/utils"
	"github.com/Jeffail/gabs"
	log "github.com/Sirupsen/logrus"
	websocket "github.com/gorilla/websocket"
)

type UiSocket struct {
	WrapperKey   string
	Connection   *websocket.Conn
	Site         *site.Site
	SiteManager  *site_manager.SiteManager
	Disconnected chan int
	MsgID        int
	sync.Mutex
}

func NewUiSocket(s *site.Site, sm *site_manager.SiteManager, wrapperKey string) *UiSocket {
	socket := UiSocket{
		WrapperKey:   wrapperKey,
		Disconnected: make(chan int),
		Site:         s,
		MsgID:        1,
		SiteManager:  sm,
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
			case event := <-socket.Site.OnChanges:
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
		log.WithFields(log.Fields{
			"site":        socket.Site.Address,
			"wrapper_key": socket.WrapperKey,
			"massage":     message,
		}).Info("Message")

		switch message.Cmd {
		case "fileQuery":
			go socket.fileQuery(message)
		case "siteDelete":
			go socket.siteDelete(message)
		case "siteInfo":
			go func(message Message) {
				// socket.Site.Wait()
				info := socket.Site.GetInfo()
				if message.Params == "" {
					return
				}
				params := message.Params.(map[string]interface{})
				if params["file_status"] != nil {
					status := "file_done"
					st := socket.Site.WaitFile(params["file_status"].(string))
					if !st {
						status = "file_failed"
					}
					info.Event = []interface{}{status, params["file_status"]}
				}
				socket.Response(message.ID, info)
			}(message)
		case "siteList":
			go socket.siteList(message)
		case "serverInfo":
			go socket.Response(message.ID, GetServerInfo())
		case "feedQuery":
			go socket.feedQuery(message)
		case "dbQuery":
			go socket.dbQuery(message)
		}
	}
}

func (socket *UiSocket) dbQuery(message Message) {
	res, err := socket.Site.DB.Query(message.Params.([]interface{})[0].(string))
	if err == nil {
		socket.Response(message.ID, res)
	}
}

func (socket *UiSocket) siteDelete(message Message) {
	socket.SiteManager.Remove(message.Params.(map[string]interface{})["address"].(string))
	socket.Notification("done", "Site deleted.")
}

func (socket *UiSocket) siteList(message Message) {
	sites := []site.SiteInfo{}
	infos, _ := socket.SiteManager.GetSites().ChildrenMap()
	for _, s := range infos {
		sites = append(sites, s.Data().(site.SiteInfo))
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
			URL:       "/",
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
		Rev:            utils.REV,
		UIIP:           "127.0.0.1",
		Platform:       "linux",
		Version:        utils.VERSION,
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
