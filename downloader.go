package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	r "math/rand"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Jeffail/gabs"
	"github.com/fatih/color"
	"github.com/google/go-querystring/query"
	bencode "github.com/jackpal/bencode-go"

	log "github.com/Sirupsen/logrus"
)

type FileTask struct {
	Filename string
	Hash     string  `json:"sha512"`
	Size     float64 `json:"size"`
	Content  []byte
}
type Tasks []FileTask

type Peers []*Peer
type Downloader struct {
	Address          string
	FreePeers        Peers
	BusyPeers        Peers
	Queue            Tasks
	Done             Tasks
	ContentRequested bool
	Content          *gabs.Container
	TotalFiles       int
	OnChanges        chan SiteEvent
	InProgress       bool
	Trackers         []string
	sync.Mutex
}

type Announce struct {
	InfoHash   string `url:"info_hash"`
	PeerID     string `url:"peer_id"`
	Port       int    `url:"port"`
	Uploaded   int    `url:"uploaded"`
	Downloaded int    `url:"downloaded"`
	Left       int    `url:"left"`
	Compact    int    `url:"compact"`
	NumWant    int    `url:"numwant"`
	Event      string `url:"event"`
}

func NewAnnounce(address string, tracker string) Announce {
	return Announce{
		InfoHash: fmt.Sprintf("%s", sha1.Sum([]byte(address))),
		PeerID:   PEER_ID,
		Port:     0, //15441,
		Uploaded: 0, Downloaded: 0,
		Left: 0, Compact: 1, NumWant: 30,
		Event: "started",
	}
}

func NewDownloader(address string) *Downloader {
	d := Downloader{
		Address:   address,
		OnChanges: make(chan SiteEvent, 10),
		Trackers:  []string{},
	}
	return &d
}

func (d *Downloader) Download(done chan int) bool {
	success := true
	green := color.New(color.FgGreen).SprintFunc()
	fmt.Println(fmt.Sprintf("Download site: %s", green(d.Address)))

	d.FreePeers = Peers{}
	d.BusyPeers = Peers{}

	dir := path.Join(DATA, d.Address)
	os.MkdirAll(dir, 0777)

	d.ContentRequested = false
	d.Queue = []FileTask{FileTask{
		Filename: "content.json",
	}}

	inbox := make(chan *Peer)
	announce := make(chan int)
	files := make(chan FileTask)
	trackers := getTrackers()
	for _, tracker := range trackers {
		go d.announceTracker(inbox, announce, tracker)
	}
	// announced := false
	tCount := 0
	fCount := 1
	d.InProgress = true
	for d.InProgress {
		select {
		case file := <-files:
			if file.Filename == "" {
				break
			}
			log.WithFields(log.Fields{
				"file": file.Filename,
			}).Infof("Task completed [%d/%d] in q: %d", fCount, len(d.Queue)+len(d.Done), len(d.Queue))
			fCount++
			go func() { d.OnChanges <- SiteEvent{"file_done", file.Filename} }()
			if len(d.Queue) > 0 {
				go func() {
					d.Lock()
					files <- d.schedileFile(d.Address)
					d.Unlock()
				}()
			} else if len(d.Done) == d.TotalFiles {
				fmt.Println("Site downloaded.")
				d.InProgress = false
				break
				return success
				// os.Exit(0)
			}
		case peer := <-inbox:
			// go func() { d.OnChanges <- 0 }()
			fmt.Println(peer.Address, peer.Port)
			go func() { d.OnChanges <- SiteEvent{"peers_added", 1} }()
			if d.peerIsKnown(peer) {
				continue
			}
			go func(peer *Peer) {
				d.connectPeer(peer)
				if peer.State != Connected {
					return
				}

				d.FreePeers = append(d.FreePeers, peer)
				if !d.ContentRequested {
					d.Lock()
					files <- d.processContent()
					d.Unlock()
				} else {
					d.Lock()
					if len(d.Queue) > 0 {
						files <- d.schedileFile(d.Address)
					} else {
						// fmt.Println("All files scheduled")
					}
					d.Unlock()
				}
			}(peer)
		case _ = <-announce:
			//TODO: waiting for connections
			tCount++
			if tCount == len(trackers) && len(d.FreePeers)+len(d.BusyPeers) == 0 {
				fmt.Println("No peers founded.")
				d.InProgress = false
				break
				return false
			}
		}
	}
	fmt.Println("Downloader finished.")
	done <- 0
	return success
	// fmt.Printf("Peers: %s", yellow(len(d.Peers)))
}

func (d *Downloader) connectPeer(peer *Peer) {
	err := peer.Connect()
	if err == nil {
		// fmt.Println(fmt.Sprintf("Connection established: %s:%d", peer.Address, peer.Port))
		peer.Ping()
	} else {
		// log.WithFields(log.Fields{
		// 	"error": err,
		// 	"peer":  peer,
		// }).Warn("Connection error")
		d.removeFreePeer(peer)
	}
}

func (d *Downloader) GetContent() (*gabs.Container, error) {
	filename := path.Join(DATA, d.Address, "content.json")
	if _, err := os.Stat(filename); err != nil {
		return nil, errors.New("Not downloaded yet")
	}
	return loadJSON(filename)
}

func (d *Downloader) processContent() FileTask {
	d.ContentRequested = true
	task := d.schedileFile(d.Address)
	content, _ := gabs.ParseJSON(task.Content)
	d.Content = content
	files, _ := content.S("files").ChildrenMap()
	for filename, child := range files {
		file := child.Data().(map[string]interface{})
		d.Queue = append(d.Queue, FileTask{
			Filename: filename,
			Hash:     file["sha512"].(string),
			Size:     file["size"].(float64),
		})
	}
	d.TotalFiles = len(files) + 1
	return task

}

func (d *Downloader) schedileFile(site string) FileTask {
	if len(d.Queue) == 0 {
		return FileTask{}
	}
	peer := d.FreePeers[0]
	task := d.Queue[0]
	filename := path.Join(DATA, site, task.Filename)
	if _, err := os.Stat(filename); err == nil && task.Filename != "content.json" {
		d.Queue = d.Queue[1:]
		d.Done = append(d.Done, task)
		return task
	}
	log.WithFields(log.Fields{
		"task": task,
		"peer": peer,
	}).Info("Requesting file")
	d.removeFreePeer(peer)
	d.BusyPeers = append(d.BusyPeers, peer)
	d.Queue = d.Queue[1:]
	file := peer.Download(site, task.Filename)
	d.Done = append(d.Done, task)
	d.removeBusyPeer(peer)
	d.FreePeers = append(d.FreePeers, peer)
	task.Content = file
	return task
}

func (d *Downloader) removeFreePeer(peer *Peer) {
	i := func() int {
		i := 0
		for _, b := range d.FreePeers {
			if b.Address == peer.Address {
				return i
			}
			i++
		}
		return -1
	}()
	if i == -1 {
		return
	}
	d.FreePeers = append(d.FreePeers[:i], d.FreePeers[i+1:]...)
}

func (d *Downloader) removeBusyPeer(peer *Peer) {
	i := func() int {
		i := 0
		for _, b := range d.BusyPeers {
			if b.Address == peer.Address {
				return i
			}
			i++
		}
		return -1
	}()
	if i == -1 {
		return
	}
	d.BusyPeers = append(d.BusyPeers[:i], d.BusyPeers[i+1:]...)
}

func (d *Downloader) peerIsKnown(peer *Peer) bool {
	for _, b := range d.FreePeers {
		if b.Address == peer.Address {
			return true
		}
	}
	return false
}

func (d *Downloader) returnLocalhost(ch chan *Peer, done chan int) {
	peer := Peer{Address: "192.168.1.38", Port: 15441}
	ch <- &peer
	done <- 0
	return
}

func (d *Downloader) announceHTTP(ch chan *Peer, done chan int, tracker string) {
	params, _ := query.Values(NewAnnounce(d.Address, tracker))
	url := fmt.Sprintf("%s?%s", tracker, params.Encode())
	log.WithFields(log.Fields{
		"tracker": tracker,
		"params":  params,
	}).Info("Announce tracker")
	resp, _ := http.Get(url)
	raw, _ := bencode.Decode(resp.Body)
	resp.Body.Close()
	data := raw.(map[string]interface{})
	peerData, _ := GetBytes(data["peers"])
	peerReader := bytes.NewReader(peerData)
	peerCount := len(peerData) / 6
	for i := 0; i < peerCount; i++ {
		peer := NewPeer(peerReader)
		if peer.Port == 0 {
			continue
		}
		ch <- peer
	}
	done <- 0
}

const (
	UDP_REQUEST_CONNECT  = 0
	UDP_REQUEST_ANNOUNCE = 1
)

func (d *Downloader) announceUDP(ch chan *Peer, done chan int, tracker string) {
	serverAddr, err := net.ResolveUDPAddr("udp", strings.Replace(tracker, "udp://", "", 1))
	connectionID := int64(0x41727101980)
	port := r.Intn(99) + 6800
	rnd := r.New(r.NewSource(time.Now().UnixNano()))
	transactionID := rnd.Int31()
	sid := "-ZN" + strconv.Itoa(os.Getpid()) + "_" + strconv.FormatInt(rnd.Int63(), 10)
	peerID := sid[0:20]
	infoHash := sha1.Sum([]byte(d.Address))
	fmt.Println(fmt.Sprintf("%x", sha1.Sum([]byte(d.Address))))
	conn, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", port))
	if err != nil {
		return
	}
	socket := conn.(*net.UDPConn)
	defer socket.Close()

	err = socket.SetDeadline(time.Now().Add(9 * time.Second))

	request := new(bytes.Buffer)
	var data = []interface{}{
		connectionID,
		int32(UDP_REQUEST_CONNECT),
		transactionID,
	}
	for _, v := range data {
		binary.Write(request, binary.BigEndian, v)
	}
	socket.WriteToUDP(request.Bytes(), serverAddr)
	buf := make([]byte, 16)
	buf2 := make([]byte, 10240)
	n, addr, err := socket.ReadFromUDP(buf)
	fmt.Println("Received ", fmt.Sprintf("%x", buf[0:n]), " from ", addr)
	if err != nil {
		fmt.Println("Error: ", err)
		return
	}
	answer := bytes.NewReader(buf)
	var resp_a int32
	var resp_t int32
	var resp_c int64
	binary.Read(answer, binary.BigEndian, &resp_a)
	binary.Read(answer, binary.BigEndian, &resp_t)
	binary.Read(answer, binary.BigEndian, &resp_c)
	fmt.Println(resp_a, resp_t, resp_c)
	announce := new(bytes.Buffer)
	data = []interface{}{
		resp_c,
		int32(UDP_REQUEST_ANNOUNCE),
		transactionID,
		infoHash,
		peerID,
		int64(0),
		int64(0),
		int64(0),
		int32(2),
		int32(0),
		int32(0),
		int32(50),
		int16(port),
	}

	for _, v := range data {
		binary.Write(announce, binary.BigEndian, v)
	}
	fmt.Println(fmt.Sprintf("%x", announce.Bytes()))
	socket.WriteToUDP(announce.Bytes(), serverAddr)
	n, addr, err = socket.ReadFromUDP(buf2)
	buf2 = buf2[0:n]
	fmt.Println("Received ", fmt.Sprintf("%x", buf2), " from ", addr, tracker)
	if err != nil {
		fmt.Println("Error: ", err)
		return
	}
	answer = bytes.NewReader(buf2)
	var a int32
	var t int32
	var i int32
	var l int32
	var s int32
	binary.Read(answer, binary.BigEndian, &a)
	binary.Read(answer, binary.BigEndian, &t)
	binary.Read(answer, binary.BigEndian, &i)
	binary.Read(answer, binary.BigEndian, &l)
	binary.Read(answer, binary.BigEndian, &s)
	for answer.Len() > 0 {
		peer := NewPeer(answer)
		if peer.Port == 0 {
			continue
		}
		ch <- peer
	}
	done <- 0
}

func (d *Downloader) announceTracker(ch chan *Peer, done chan int, tracker string) {
	// d.returnLocalhost(ch, done)
	// yellow := color.New(color.FgYellow).SprintFunc()
	// red := color.New(color.FgRed).SprintFunc()
	d.Trackers = append(d.Trackers, tracker)
	if strings.HasPrefix(tracker, "http://") {
		d.announceHTTP(ch, done, tracker)
	} else if strings.HasPrefix(tracker, "udp://") {
		d.announceUDP(ch, done, tracker)
	}
}
