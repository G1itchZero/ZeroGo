package downloader

import (
	"errors"
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/G1itchZero/zeronet-go/events"
	"github.com/G1itchZero/zeronet-go/interfaces"
	"github.com/G1itchZero/zeronet-go/peer_manager"
	"github.com/G1itchZero/zeronet-go/tasks"
	"github.com/G1itchZero/zeronet-go/utils"
	"github.com/Jeffail/gabs"
	"github.com/fatih/color"

	log "github.com/Sirupsen/logrus"
)

type Downloader struct {
	Address          string
	Peers            *peer_manager.PeerManager
	Tasks            tasks.Tasks
	ContentRequested bool
	Content          *gabs.Container
	TotalFiles       int
	StartedTasks     int
	OnChanges        chan events.SiteEvent
	InProgress       bool
	tasksDone        chan *tasks.FileTask
	trackersDone     int
	filesDone        int
	done             chan int
	sync.Mutex
}

func NewDownloader(address string) *Downloader {
	d := Downloader{
		Peers:        peer_manager.NewPeerManager(address),
		Address:      address,
		OnChanges:    make(chan events.SiteEvent, 400),
		tasksDone:    make(chan *tasks.FileTask, 100),
		trackersDone: 0,
		filesDone:    1,
		StartedTasks: 0,
	}
	return &d
}

func (d *Downloader) handleFile(file *tasks.FileTask) {
	if file == nil {
		return
	}
	d.filesDone++
	d.OnChanges <- events.SiteEvent{"file_done", file.Filename}
	log.WithFields(log.Fields{
		"file": file.Filename,
	}).Infof("Task completed [%d/%d] in q: %d", d.filesDone, d.TotalFiles, d.PendingTasks())

	go func() {
		pt := d.PendingTasks()
		done := d.FinishedTasks()
		fmt.Println(done, d.TotalFiles, pt)
		if pt > 0 {
			d.tasksDone <- d.scheduleFile(d.Address)
		} else if done >= d.TotalFiles {
			d.InProgress = false
			d.done <- 0
			go d.Peers.Stop()
		}
	}()
}

func (d *Downloader) handlePeer(p interfaces.IPeer) {
	if !d.ContentRequested {
		d.tasksDone <- d.processContent()
	}
}

func (d *Downloader) handleAnnounce(peers int) {
	//TODO: waiting for connections
	go func() { d.OnChanges <- events.SiteEvent{"peers_added", peers} }()
	d.trackersDone++
	if d.trackersDone == len(d.Peers.Trackers) && d.Peers.Count == 0 {
		fmt.Println("No peers founded.")
		d.InProgress = false
		return
	}
}

func (d *Downloader) Download(done chan int) bool {
	success := true
	green := color.New(color.FgGreen).SprintFunc()
	fmt.Println(fmt.Sprintf("Download site: %s", green(d.Address)))

	dir := path.Join(utils.GetDataPath(), d.Address)
	os.MkdirAll(dir, 0777)

	d.ContentRequested = false
	d.Tasks = tasks.Tasks{&tasks.FileTask{
		Filename: "content.json",
		Site:     d.Address,
	}}

	d.done = make(chan int, 100)
	go d.Peers.Announce()
	d.InProgress = true
	for d.InProgress {
		select {
		case file := <-d.tasksDone:
			go d.handleFile(file)
		case peer := <-d.Peers.OnPeers:
			go d.handlePeer(peer)
		case peers := <-d.Peers.OnAnnounce:
			go d.handleAnnounce(peers)
		case <-d.done:
			break
		}
	}
	fmt.Println(green("Site downloaded."))
	d.filesDone = 0
	done <- 0
	return success
	// fmt.Printf("Peers: %s", yellow(len(d.Peers)))
}

func (d *Downloader) GetContent() (*gabs.Container, error) {
	filename := path.Join(utils.GetDataPath(), d.Address, "content.json")
	if _, err := os.Stat(filename); err != nil {
		return nil, errors.New("Not downloaded yet")
	}
	return utils.LoadJSON(filename)
}

func (d *Downloader) processContent() *tasks.FileTask {
	d.ContentRequested = true
	task := d.scheduleFile(d.Address)
	content, _ := gabs.ParseJSON(task.Content)
	d.Content = content
	files, _ := content.S("files").ChildrenMap()
	for filename, child := range files {
		file := child.Data().(map[string]interface{})
		d.Tasks = append(d.Tasks, &tasks.FileTask{
			Filename: filename,
			Hash:     file["sha512"].(string),
			Size:     file["size"].(float64),
			Site:     d.Address,
		})
	}
	d.TotalFiles = len(files) + 1 //content.json
	return task

}

func (d *Downloader) scheduleFile(site string) *tasks.FileTask {
	d.StartedTasks++
	if d.PendingTasks() == 0 {
		return nil
	}
	peer := d.Peers.Get()
	if peer == nil {
		return nil
	}

	for _, task := range d.Tasks {
		if len(task.Content) == 0 {
			filename := path.Join(utils.GetDataPath(), site, task.Filename)
			if _, err := os.Stat(filename); err == nil && task.Filename != "content.json" {
				log.WithFields(log.Fields{
					"task": task.Filename,
				}).Info("File from disk")
				return task
			}
			log.WithFields(log.Fields{
				"task": task.Filename,
				"peer": peer.Address,
			}).Info("Requesting file")
			task.AddPeer(peer)
			return task
		}
	}
	return nil
}

func (d *Downloader) PendingTasks() int {
	n := 0
	for _, task := range d.Tasks {
		if len(task.Content) == 0 {
			n++
		}
	}
	return n
}

func (d *Downloader) FinishedTasks() int {
	n := 0
	for _, task := range d.Tasks {
		if len(task.Content) != 0 {
			n++
		}
	}
	return n
}
