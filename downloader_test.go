package downloader

import (
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"
)

func Test_NewFile(t *testing.T) {
	fileDl, err := NewFileDl("", "http://packages.linuxdeepin.com/ubuntu/dists/devel/main/binary-amd64/Packages.bz2", -1, "/tmp", "")
	if err != nil {
		log.Println(err)
	}

	fileDl.OnStart(func(id string) {
		fmt.Println(GetDownloader(id).File.Name, "download started")
	})

	exit := make(chan bool)

	fileDl.OnFinish(func(id string) {
		exit <- true
	})

	fileDl.OnError(func(id string, errCode int, errStr string) {
		log.Println(GetDownloader(id).File.Name, errStr)
	})

	fmt.Printf("%+v\n", fileDl)

	fileDl.Start()
	for {
		select {
		case <-exit:
			fmt.Println("\n", fileDl.File.Name, "download finished")
			os.Exit(0)
		default:
			time.Sleep(time.Second * 1)
			status := fileDl.GetStatus()
			var i = float64(status.Downloaded) / float64(fileDl.File.Size) * 100
			h := strings.Repeat("=", int(i/2)) + strings.Repeat(" ", 50-int(i/2))
			fmt.Printf("\r%.0f%%[%s] %v byte/s          ", i, h, status.Speeds)
			os.Stdout.Sync()
		}
	}
}
