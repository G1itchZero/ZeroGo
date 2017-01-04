package peer_manager

import (
	"bytes"
	"container/heap"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	r "math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/G1itchZero/ZeroGo/peer"
	"github.com/G1itchZero/ZeroGo/utils"
	log "github.com/Sirupsen/logrus"
	"github.com/google/go-querystring/query"
	bencode "github.com/jackpal/bencode-go"
)

type Peers []*peer.Peer

type PeerManager struct {
	Address    string
	Peers      Peers
	Count      int
	Trackers   []string
	OnPeers    chan *peer.Peer
	OnAnnounce chan int
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

func NewPeerManager(address string) *PeerManager {
	pm := PeerManager{
		Address:    address,
		Peers:      Peers{},
		OnPeers:    make(chan *peer.Peer, 100),
		OnAnnounce: make(chan int),
	}
	heap.Init(&pm.Peers)
	return &pm
}

func (pm *PeerManager) GetActivePeers() Peers {
	peers := Peers{}
	for _, peer := range pm.Peers {
		peers = append(peers, peer)
	}
	return peers
}

func (pm *PeerManager) Stop() {
	log.Println("peers stopping")
	log.Fatal("peers stopped")
	for _, peer := range pm.Peers {
		go peer.Stop()
	}
	pm.Peers = Peers{}
	log.Fatal("peers stopped")
}

func (pm *PeerManager) Get() *peer.Peer {
	for len(pm.Peers) == 0 {
		time.Sleep(time.Duration(time.Millisecond * 100))
	}
	pm.Lock()
	p := heap.Pop(&pm.Peers)
	if p == nil {
		p = pm.Get()
	}
	pm.Count--
	pm.Unlock()
	return p.(*peer.Peer)
}

func (pm *PeerManager) Announce() {
	pm.Trackers = utils.GetTrackers()
	for _, tracker := range pm.Trackers {
		go func(tracker string) {
			peers := pm.announceTracker(tracker)
			c := 0
			for _, p := range peers {
				if pm.peerIsKnown(p) {
					continue
				}
				c++
				go func(p *peer.Peer) {
					err := pm.connectPeer(p)
					if err != nil {
						return
					}
					heap.Push(&pm.Peers, p)
					pm.Count++
					pm.OnPeers <- p
				}(p)
			}
			pm.OnAnnounce <- c
			log.WithFields(log.Fields{
				"tracker": tracker,
				"peers":   c,
			}).Debug("New peers added")
		}(tracker)
	}
}

func (pm *PeerManager) connectPeer(peer *peer.Peer) error {
	err := peer.Connect()
	if err == nil {
		log.WithFields(log.Fields{
			"peer": peer,
		}).Debug("Peer connected")
		peer.Ping()
	} else {
		// log.WithFields(log.Fields{
		// 	"error": err,
		// 	"peer":  peer,
		// }).Warn("Connection error")
		pm.removePeer(peer)
	}
	return err
}

func NewAnnounce(address string, tracker string) Announce {
	return Announce{
		InfoHash: fmt.Sprintf("%s", sha1.Sum([]byte(address))),
		PeerID:   utils.GetPeerID(),
		Port:     0, //15441,
		Uploaded: 0, Downloaded: 0,
		Left: 0, Compact: 1, NumWant: 30,
		Event: "started",
	}
}

func (pm *PeerManager) removePeer(peer *peer.Peer) {
	i := func() int {
		i := 0
		for _, b := range pm.Peers {
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
	pm.Peers = append(pm.Peers[:i], pm.Peers[i+1:]...)
}

func (pm *PeerManager) peerIsKnown(peer *peer.Peer) bool {
	if peer.Address == "0.0.0.0" || peer.Address == utils.GetExternalIP() || peer.Address == "9.12.0.6" {
		return true
	}
	for _, b := range pm.Peers {
		if b.Address == peer.Address {
			return true
		}
	}
	return false
}

func (pm *PeerManager) announceHTTP(tracker string) Peers {
	params, _ := query.Values(NewAnnounce(pm.Address, tracker))
	url := fmt.Sprintf("%s?%s", tracker, params.Encode())
	log.WithFields(log.Fields{
		"tracker": tracker,
		"params":  params,
	}).Debug("Announce HTTP tracker")
	resp, err := http.Get(url)
	if err != nil {
		log.Warn(fmt.Errorf("Announce error: %v", err))
		return Peers{}
	}
	raw, err := bencode.Decode(resp.Body)
	if err != nil {
		log.Warn(fmt.Errorf("Announce error: %v", err))
		return Peers{}
	}
	resp.Body.Close()
	data := raw.(map[string]interface{})
	if data["peers"] == nil {
		return Peers{}
	}
	peerData, _ := utils.GetBytes(data["peers"])
	peerReader := bytes.NewReader(peerData)
	peerCount := len(peerData) / 6
	peers := Peers{}
	for i := 0; i < peerCount; i++ {
		peer := peer.NewPeer(peerReader, pm.OnPeers)
		if peer == nil {
			continue
		}
		peers = append(peers, peer)
	}
	return peers
}

const (
	UDP_REQUEST_CONNECT  = 0
	UDP_REQUEST_ANNOUNCE = 1
)

func (pm *PeerManager) connectUDPTracker(socket *net.UDPConn, serverAddr *net.UDPAddr, transactionID int32) (connectionID int64, err error) {
	connectionID = int64(0x41727101980)
	request := new(bytes.Buffer)
	var data = []interface{}{
		connectionID,
		uint32(UDP_REQUEST_CONNECT),
		transactionID,
	}
	for _, v := range data {
		binary.Write(request, binary.BigEndian, v)
	}
	socket.WriteToUDP(request.Bytes(), serverAddr)
	buf := make([]byte, 16)
	_, _, err = socket.ReadFromUDP(buf)
	if err != nil {
		return 0, err
	}
	answer := bytes.NewReader(buf)
	var action int32
	binary.Read(answer, binary.BigEndian, &action)
	binary.Read(answer, binary.BigEndian, &transactionID)
	binary.Read(answer, binary.BigEndian, &connectionID)
	return connectionID, nil
	// fmt.Println("Received ", fmt.Sprintf("%x", buf[0:n]), " from ", addr)
}

func (pm *PeerManager) announceUDPTracker(socket *net.UDPConn, serverAddr *net.UDPAddr, transactionID int32, connectionID int64, port int32) (peers Peers, err error) {
	peerID := []byte(utils.GetPeerID()[0:20])
	infoHash := sha1.Sum([]byte(pm.Address))

	announce := new(bytes.Buffer)
	data := []interface{}{
		connectionID,
		uint32(UDP_REQUEST_ANNOUNCE),
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
	log.WithFields(log.Fields{
		"tracker": serverAddr.String(),
		"params":  fmt.Sprintf("%x", announce.Bytes()),
	}).Debug("Announce UDP tracker")
	socket.WriteToUDP(announce.Bytes(), serverAddr)
	buf2 := make([]byte, 10240)
	n, _, err := socket.ReadFromUDP(buf2)
	buf2 = buf2[0:n]
	if err != nil {
		// fmt.Println("Error: ", err)
		return Peers{}, err
	}
	answer := bytes.NewReader(buf2)
	var a uint32
	var t uint32
	var i uint32
	var l uint32
	var s uint32
	binary.Read(answer, binary.BigEndian, &a)
	binary.Read(answer, binary.BigEndian, &t)
	binary.Read(answer, binary.BigEndian, &i)
	binary.Read(answer, binary.BigEndian, &l)
	binary.Read(answer, binary.BigEndian, &s)
	peers = Peers{}
	for answer.Len() > 0 {
		peer := peer.NewPeer(answer, pm.OnPeers)
		if peer == nil {
			continue
		}
		peers = append(peers, peer)
	}
	return peers, nil
}

func (pm *PeerManager) announceUDP(tracker string) Peers {
	serverAddr, _ := net.ResolveUDPAddr("udp", strings.Replace(tracker, "udp://", "", 1))
	rnd := r.New(r.NewSource(time.Now().UnixNano()))
	transactionID := rnd.Int31()
	port := int32(rnd.Intn(99) + 6800)

	conn, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", port))
	if err != nil {
		return Peers{}
	}
	socket := conn.(*net.UDPConn)
	defer socket.Close()

	socket.SetDeadline(time.Now().Add(20 * time.Second))

	connectionID, err := pm.connectUDPTracker(socket, serverAddr, transactionID)

	if err != nil {
		// fmt.Println("Error: ", err)
		return Peers{}
	}

	peers, err := pm.announceUDPTracker(socket, serverAddr, transactionID, connectionID, port)

	if err != nil {
		// fmt.Println("Error: ", err)
		return Peers{}
	}
	return peers

}

func (pm *PeerManager) announceTracker(tracker string) Peers {
	pm.Trackers = append(pm.Trackers, tracker)
	if strings.HasPrefix(tracker, "http://") {
		return pm.announceHTTP(tracker)
	} else if strings.HasPrefix(tracker, "udp://") {
		return pm.announceUDP(tracker)
	}
	return Peers{}
}

func (pq Peers) Len() int { return len(pq) }

func (pq Peers) Less(i, j int) bool {
	return pq[i].ActiveTasks < pq[j].ActiveTasks
}

func (pq Peers) Swap(i, j int) {
	// pq[i], pq[j] = pq[j], pq[i]
}

func (pq *Peers) Push(x interface{}) {
	item := x.(*peer.Peer)
	*pq = append(*pq, item)
}

func (pq *Peers) Pop() interface{} {
	old := *pq
	n := len(old)
	if n == 0 {
		return nil
	}
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}
