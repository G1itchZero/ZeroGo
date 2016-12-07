package main

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path"
	"time"

	"github.com/fatih/color"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo"
)

type Server struct {
	Port    int
	Sockets map[string]*UiSocket
	Sites   *SiteManager
	Queue   map[string]*Site
}

func NewServer(port int, sites *SiteManager) *Server {
	server := Server{
		Port:    port,
		Sockets: map[string]*UiSocket{},
		Sites:   sites,
		Queue:   map[string]*Site{},
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
	e.Static("/uimedia", "./media")

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
		url = ZN_HOMEPAGE
	}
	if url == "favicon.ico" {
		return nil
	}
	site := s.Sites.Get(url)
	fmt.Println(fmt.Sprintf("> %s", yellow(url)))
	s.Queue[url] = site
	wrapper := NewWrapper(site)
	err := wrapper.Render(ctx)
	socket := NewUiSocket(site, wrapper.Key)
	s.Sockets[wrapper.Key] = socket
	return err
}

func (s *Server) serveInner(ctx echo.Context) error {
	name := ctx.Param("site")
	root := path.Join(DATA, name)
	filename := path.Join(root, "index.html")
	site := s.Queue[name]
	site.Wait()
	return ctx.File(filename)
}

func (s *Server) serveInnerStatic(ctx echo.Context) error {
	site, _ := ctx.Cookie("site")
	url := ctx.Param("*")
	root := path.Join(DATA, site.Value)
	filename := path.Join(root, url)
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
