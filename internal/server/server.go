package server

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	hlog "github.com/cloudwego/hertz/pkg/common/hlog"

	"github.com/yourname/go-research/internal/config"
	"github.com/yourname/go-research/internal/llm"
)

type Server struct {
	cfg *config.Config
	llm *llm.Client
	h   *server.Hertz
}

func New(cfg *config.Config, llmClient *llm.Client) *Server {
	h := server.Default(server.WithHostPorts(cfg.ServerAddr))
	s := &Server{cfg: cfg, llm: llmClient, h: h}
	s.routes()
	return s
}

func (s *Server) Run() error {
	hlog.Infof("listening on %s", s.cfg.ServerAddr)
	return s.h.Run()
}
