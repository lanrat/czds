package main

import (
	"log"
	"time"
)

func v(format string, v ...interface{}) {
	if *verbose {
		log.Printf(format, v...)
	}
}

func expiredTime(t time.Time) string {
	if t.Unix() > 0 {
		return t.Format(time.ANSIC)
	}
	return ""
}
