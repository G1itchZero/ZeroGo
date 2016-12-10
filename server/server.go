package server

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path"
	"time"

	"github.com/G1itchZero/zeronet-go/site"
	"github.com/G1itchZero/zeronet-go/socket"
	"github.com/G1itchZero/zeronet-go/utils"

	"github.com/G1itchZero/zeronet-go/site_manager"
	"github.com/fatih/color"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo"
)

type Server struct {
	Port    int
	Sockets map[string]*socket.UiSocket
	Sites   *site_manager.SiteManager
	Queue   map[string]*site.Site
}

func NewServer(port int, sites *site_manager.SiteManager) *Server {
	server := Server{
		Port:    port,
		Sockets: map[string]*socket.UiSocket{},
		Sites:   sites,
		Queue:   map[string]*site.Site{},
	}
	return &server
}

func InnerMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(ctx echo.Context) error {
		if ctx.Path() == "/inner/:site" {
			site := ctx.Param("site")
			cookie := new(http.Cookie)
			cookie.Name = "site"
			cookie.Value = site
			cookie.Expires = time.Now().Add(24 * time.Hour)
			ctx.SetCookie(cookie)
		}
		return next(ctx)
	}
}

var (
	upgrader = websocket.Upgrader{}
)

func (s *Server) socketHandler(c echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	wrapperKey := c.QueryParam("wrapper_key")
	if err != nil {
		return err
	}
	defer ws.Close()

	socket, ok := s.Sockets[wrapperKey]

	if ws != nil && ok {
		socket.Serve(ws)
	}
	return errors.New("Socket closed")
}

func (s *Server) Serve() {
	e := echo.New()
	e.Static("/uimedia", path.Join(utils.GetDataPath(), utils.ZN_UPDATE, "ZeroNet", "src", "Ui", "media"))

	inner := e.Group("/inner")
	inner.Use(InnerMiddleware)
	inner.GET("/:site", s.serveInner)
	inner.GET("/*", s.serveInnerStatic)

	e.GET("/Websocket", s.socketHandler)
	// e.GET("/favicon.ico", s.serveInnerStatic, InnerMiddleware)
	e.GET("/:url/", s.serveWrapper)
	e.GET("/:url", s.serveWrapper)
	e.GET("/", s.serveWrapper)

	e.Logger.Fatal(e.Start(fmt.Sprintf(":%d", s.Port)))
}

func (s *Server) serveWrapper(ctx echo.Context) error {
	yellow := color.New(color.FgYellow).SprintFunc()
	url := ctx.Param("url")
	if url == "" {
		url = utils.ZN_HOMEPAGE
	}
	if url == "favicon.ico" {
		return nil
	}
	st := s.Sites.Get(url)
	fmt.Println(fmt.Sprintf("> %s", yellow(st.Address)))
	s.Queue[url] = st
	wrapper := NewWrapper(st)
	err := wrapper.Render(ctx)
	socket := socket.NewUiSocket(st, s.Sites, wrapper.Key)
	s.Sockets[wrapper.Key] = socket
	return err
}

func (s *Server) serveInner(ctx echo.Context) error {
	name := ctx.Param("site")
	root := path.Join(utils.GetDataPath(), name)
	filename := path.Join(root, "index.html")
	site := s.Queue[name]
	site.WaitFile("index.html")
	return ctx.File(filename)
}

func (s *Server) serveInnerStatic(ctx echo.Context) error {
	name, _ := ctx.Cookie("site")
	url := ctx.Param("*")
	root := path.Join(utils.GetDataPath(), name.Value)
	filename := path.Join(root, url)
	site := s.Queue[name.Value]
	site.WaitFile(url)
	return ctx.File(filename)
}

func serveStatic(ctx echo.Context, p string) error {
	fmt.Println(p)
	return ctx.File(p)
}

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}
