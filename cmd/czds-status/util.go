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
	if !t.IsZero() {
		return t.Format(time.ANSIC)
	}
	return ""
}
