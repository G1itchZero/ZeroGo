package tasks

import (
	"fmt"
	"io/ioutil"
	"log"
	"path"

	"github.com/G1itchZero/zeronet-go/events"
	"github.com/G1itchZero/zeronet-go/interfaces"
	"github.com/G1itchZero/zeronet-go/utils"
)

type Tasks []*FileTask
type FileTask struct {
	Site       string
	Filename   string
	Hash       string  `json:"sha512"`
	Size       float64 `json:"size"`
	Downloaded float64
	Content    []byte
	Peers      []interfaces.IPeer
	Started    bool
	Done       bool
	OnChanges  chan events.SiteEvent
	Priority   int
}

func NewTask(filename string, hash string, size float64, site string, ch chan events.SiteEvent) *FileTask {
	p := 0
	if filename == "content.json" {
		p = 9999
	} else if filename == "index.html" {
		p = 9990
	}
	task := FileTask{
		Filename:  filename,
		Hash:      hash,
		Size:      size,
		Site:      site,
		OnChanges: ch,
		Priority:  p,
	}
	return &task
}

func (task *FileTask) String() string {
	return fmt.Sprintf("<Task: %s [%d] (peers: %d) Done: %v>", task.Filename, task.Priority, len(task.Peers), task.Done)
}

func (task *FileTask) GetFilename() string {
	return task.Filename
}

func (task *FileTask) GetContent() []byte {
	return task.Content
}

func (task *FileTask) SetContent(content []byte) {
	// if len(task.Content) > 0 {
	// 	return
	// }
	filename := path.Join(utils.GetDataPath(), task.Site, task.Filename)
	task.Content = content
	err := ioutil.WriteFile(filename, task.Content, 0644)
	if err != nil {
		log.Fatal(task, err)
	}
	task.Finish()
}

func (task *FileTask) GetSite() string {
	return task.Site
}

func (task *FileTask) Start() {
	task.Started = true
}

func (task *FileTask) Finish() {
	if !task.Done {
		task.Done = true
		// task.OnChanges <- events.SiteEvent{Type: "file_done", Payload: task.Filename}
		fmt.Println("finished", task)
		task.Priority = -1
		// go func() { task.OnChanges <- events.SiteEvent{Type: "file_done", Payload: task.Filename} }()
	}
}

func (task *FileTask) AddPeer(p interfaces.IPeer) {
	if task.Peers == nil {
		task.Peers = []interfaces.IPeer{}
	}
	task.Peers = append(task.Peers, p)
	p.AddTask(task)
}

func (a Tasks) Len() int           { return len(a) }
func (a Tasks) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a Tasks) Less(i, j int) bool { return a[i].Priority > a[j].Priority }
