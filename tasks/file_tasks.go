package tasks

import (
	"fmt"
	"io/ioutil"
	"path"
	"crypto/sha512"
	"os"
	"io"
	"bytes"

	"github.com/G1itchZero/ZeroGo/events"
	"github.com/G1itchZero/ZeroGo/interfaces"
	"github.com/G1itchZero/ZeroGo/utils"
	log "github.com/Sirupsen/logrus"
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
	Success    bool
	FullPath   string
  Location int
	Stream io.Writer
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
		FullPath: path.Join(utils.GetDataPath(), site, filename),
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

func (task *FileTask) GetSize() int64 {
	return int64(task.Size)
}

func (task *FileTask) AppendContent(content []byte, location int) {
	if task.Stream == nil {
    var err error
		task.Stream, err = os.Create(task.FullPath)
	  if err != nil {
	    // panic?
	  }
	}
  if (location == 0 && task.Location == 0) || location > task.Location {
    task.Location = location
    // defer out.Close()
    io.Copy(task.Stream, bytes.NewReader(content))
  }
}

func (task *FileTask) SetContent(content []byte) {
	// if len(task.Content) > 0 {
	// 	return
	// }
	task.Content = content
	hash := fmt.Sprintf("%x", sha512.Sum512(task.Content))[0:64]
	err := ioutil.WriteFile(task.FullPath, task.Content, 0644)
	if task.Hash != "" && task.Hash != hash {
		fmt.Printf("Size error '%s': %d != %d\n", task.FullPath, int(task.Size), len(task.Content))
		log.Fatal(fmt.Errorf("Hash error '%s': %s != %s", task.FullPath, task.Hash, hash))
	}
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

func (task *FileTask) GetStarted() bool {
	return task.Started
}

func (task *FileTask) Finish() {
	if !task.Done {
		task.Done = true
		task.Success = true
		// task.OnChanges <- events.SiteEvent{Type: "file_done", Payload: task.Filename}
		log.WithFields(log.Fields{
			"task": task,
		}).Debug("Finished")
		task.Priority = -1
	}
}

func (task *FileTask) AddPeer(p interfaces.IPeer) error {
	if task.Peers == nil {
		task.Peers = []interfaces.IPeer{}
	}
	task.Peers = append(task.Peers, p)
	return p.AddTask(task)
}

func (a Tasks) Len() int           { return len(a) }
func (a Tasks) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a Tasks) Less(i, j int) bool { return a[i].Priority > a[j].Priority }
