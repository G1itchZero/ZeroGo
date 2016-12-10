package downloader

import (
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"sync"
	"time"

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
	Files            map[string]*tasks.FileTask
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
		Files:        map[string]*tasks.FileTask{},
		trackersDone: 0,
		filesDone:    1,
		StartedTasks: 0,
	}
	return &d
}

// func (d *Downloader) handleFile(file *tasks.FileTask) {
// 	if file == nil {
// 		return
// 	}
// 	d.filesDone++
// 	d.OnChanges <- events.SiteEvent{"file_done", file.Filename}
// 	log.WithFields(log.Fields{
// 		"file": file.Filename,
// 	}).Infof("Task completed [%d/%d] in q: %d", d.filesDone, d.TotalFiles, d.PendingTasks())
//
// 	go func() {
// 		pt := d.PendingTasks()
// 		done := d.FinishedTasks()
// 		fmt.Println(done, d.TotalFiles, pt)
// 		if pt > 0 {
// 			d.tasksDone <- d.ScheduleFile()
// 		} else if done >= d.TotalFiles {
// 			d.InProgress = false
// 			d.done <- 0
// 			go d.Peers.Stop()
// 		}
// 	}()
// }

func (d *Downloader) Download(done chan int) bool {
	green := color.New(color.FgGreen).SprintFunc()
	fmt.Println(fmt.Sprintf("Download site: %s", green(d.Address)))

	dir := path.Join(utils.GetDataPath(), d.Address)
	os.MkdirAll(dir, 0777)

	d.ContentRequested = false
	d.Tasks = tasks.Tasks{tasks.NewTask("content.json", "", 0, d.Address, d.OnChanges)}

	go d.Peers.Announce()
	d.processContent()
	log.Println(fmt.Sprintf("Files in queue: %s", green(len(d.Tasks)-1)))
	sort.Sort(d.Tasks)
	for _, task := range d.Tasks {
		go d.ScheduleFile(task)
	}
	for {
		select {
		case p := <-d.Peers.OnPeers:
			sort.Sort(d.Tasks)
			n := 0
			t := d.Tasks[n]
			for t.Done {
				n++
				if n >= len(d.Tasks) {
					n = -1
					break
				}
				t = d.Tasks[n]
			}
			if n >= 0 {
				fmt.Println("new peer", p, "get", t)
				go d.ScheduleFileForPeer(t, p)
			}
		}
	}
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
	task := d.ScheduleFile(d.Tasks[0])
	content, _ := gabs.ParseJSON(task.Content)
	d.Content = content
	files, _ := content.S("files").ChildrenMap()
	for filename, child := range files {
		file := child.Data().(map[string]interface{})
		t := tasks.NewTask(filename, file["sha512"].(string), file["size"].(float64), d.Address, d.OnChanges)
		d.Tasks = append(d.Tasks, t)
		d.Files[t.Filename] = t
		fmt.Println("add", filename, t)
	}
	d.TotalFiles = len(files) + 1 //content.json
	return task

}

func (d *Downloader) ScheduleFileForPeer(task *tasks.FileTask, peer interfaces.IPeer) *tasks.FileTask {
	// filename := path.Join(utils.GetDataPath(), d.Address, task.Filename)
	// if _, err := os.Stat(filename); err == nil && task.Filename != "content.json" {
	// 	log.WithFields(log.Fields{
	// 		"task": task.Filename,
	// 	}).Info("File from disk")
	// 	task.Start()
	// 	task.Finish()
	// 	return task
	// }
	log.WithFields(log.Fields{
		"task": task.Filename,
		"peer": peer.GetAddress(),
	}).Info("Requesting file")
	task.AddPeer(peer)
	return task
}

func (d *Downloader) ScheduleFile(task *tasks.FileTask) *tasks.FileTask {
	d.StartedTasks++
	if d.PendingTasks() == 0 {
		return nil
	}
	peer := d.Peers.Get()

	for peer == nil {
		peer = d.Peers.Get()
		time.Sleep(100)
	}
	return d.ScheduleFileForPeer(task, peer)
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
