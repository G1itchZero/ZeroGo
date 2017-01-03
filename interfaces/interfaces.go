package interfaces

type ISite interface {
}

type ITask interface {
	GetSite() string
	GetFilename() string
	GetContent() []byte
	GetStarted() bool
	GetDone() bool
	AppendContent([]byte, int)
	GetSize() int64
	Start()
	Check()
	Finish()
}
type IPeer interface {
	AddTask(ITask) error
	GetAddress() string
}
