package server

import "net/http"

func (s *Server) Router() *http.ServeMux { return s.router }