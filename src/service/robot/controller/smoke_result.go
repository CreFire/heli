package controller

import "time"

type SmokeResult struct {
	Module   string
	Step     string
	Received bool
	Success  bool
	Summary  string
	Updated  time.Time
}