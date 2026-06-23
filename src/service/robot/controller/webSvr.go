package controller

import (
	"fmt"
	"game/deps/xlog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type WebSvr struct {
	mux *http.ServeMux
	agt *http.Server
}

func (svr *WebSvr) RegisterHandler(pattern string, f http.HandlerFunc) {
	svr.mux.HandleFunc(pattern, f)
}

func (svr *WebSvr) Start(addr string) error {
	svr.agt = &http.Server{
		Addr:         addr,
		Handler:      svr.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	go func() {
		xlog.Infof("start to deploy http server addr=[%v]...", addr)
		if err := svr.agt.Serve(ln); err != nil && err != http.ErrServerClosed {
			fmt.Printf("[web-svr] quit : %s\n", err.Error())
		}
	}()
	return nil
}

func (svr *WebSvr) Stop() {
	xlog.Infof("[web-svr] start to stop ...")
	if svr.agt != nil {
		if err := svr.agt.Close(); err != nil {
			fmt.Printf("[web-svr] close failed : %s\n", err.Error())
		} else {
			xlog.Infof("[web-svr] stopped.")
		}
	}
}

func NewWebSvr() *WebSvr {
	return &WebSvr{
		mux: http.NewServeMux(),
	}
}

func (svr *WebSvr) RegisterStatic(prefix, dir string) {
	if svr == nil || svr.mux == nil || prefix == "" || dir == "" {
		return
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	fs := http.FileServer(http.Dir(dir))
	svr.mux.Handle(prefix, http.StripPrefix(prefix, fs))
}

func (svr *WebSvr) RegisterFile(path, file string) {
	if svr == nil || svr.mux == nil || path == "" || file == "" {
		return
	}
	svr.mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, file)
	})
}

// --=============================================================--
func (r *RobotServer) RegisterWebHandler() {
	r.webSvr.RegisterHandler("/getAllC2S", GetAllC2S)
	r.webSvr.RegisterHandler("/queryInfo", QueryInfo)
	r.webSvr.RegisterHandler("/sendMessage", SendMessage)
	r.webSvr.RegisterHandler("/getStatistics", getStatistics)
	r.webSvr.RegisterHandler("/setRobotNum", setRobotNum)
	r.webSvr.RegisterHandler("/setModeWeight", setModeWeight)
	r.webSvr.RegisterHandler("/getAllMsg", getAllMsg)
	r.webSvr.RegisterHandler("/stopRobot", stopRobot)
	r.webSvr.RegisterHandler("/runMode", runMode)
	r.webSvr.RegisterHandler("/api/smokeResults", getSmokeResults)
	r.webSvr.RegisterHandler("/robot/api/smokeResults", getSmokeResults)
	r.registerRobotWeb()
}

func (r *RobotServer) registerRobotWeb() {
	if r == nil || r.webSvr == nil {
		return
	}
	webDir := robotWebDir()
	if webDir == "" {
		WarnfLimited("robot_web_dir_missing", "robot web dir missing")
		return
	}
	indexFile := filepath.Join(webDir, "index.html")
	if _, err := os.Stat(indexFile); err == nil {
		r.webSvr.RegisterFile("/", indexFile)
	}
	for _, asset := range []string{"app.js", "style.css"} {
		file := filepath.Join(webDir, asset)
		if _, err := os.Stat(file); err == nil {
			r.webSvr.RegisterFile("/"+asset, file)
		}
	}
	r.webSvr.RegisterHandler("/robot", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/robot/", http.StatusMovedPermanently)
	})
	r.webSvr.RegisterStatic("/robot/", webDir)
}

func robotWebDir() string {
	candidates := []string{
		filepath.Join("src", "service", "robot", "web"),
		filepath.Join("web"),
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(file), "..", "web"))
	}
	for _, dir := range candidates {
		if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
			return dir
		}
	}
	return ""
}

func (r *RobotServer) registerToolWeb() {
	if r == nil || r.webSvr == nil {
		return
	}
	webDir := filepath.Join("tools", "web")
	if _, err := os.Stat(webDir); err != nil {
		WarnfLimited("robot_tool_web_dir_missing", "robot tool web dir missing: %s err=%v", webDir, err)
		return
	}
	indexFile := filepath.Join(webDir, "index.html")
	if _, err := os.Stat(indexFile); err == nil {
		r.webSvr.RegisterFile("/", indexFile)
	}
	for _, sub := range []string{"js", "css", "images"} {
		dir := filepath.Join(webDir, sub)
		if _, err := os.Stat(dir); err == nil {
			r.webSvr.RegisterStatic("/"+sub+"/", dir)
		}
	}
	r.webSvr.RegisterHandler("/server.js", serveRobotToolServerJS)
}

func serveRobotToolServerJS(w http.ResponseWriter, r *http.Request) {
	setupCors(w)
	host := r.Host
	if host == "" {
		host = "127.0.0.1:7000"
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = fmt.Fprintf(w, `getServer({
    "checkServer": false,
    "timeOut": 7000,
    "serverList": [
        {
            "text": "机器人",
            "value": "http://%s/"
        }
    ]
})`, host)
}
