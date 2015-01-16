package downloader

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func Test_NewFile(t *testing.T) {
	fileDl, err := NewFileDl("", "http://golangtc.com/static/go/go1.4.1.src.tar.gz", -1, "/tmp", "")
	if err != nil {
		log.Println(err)
	}

	exit := make(chan bool)
	var wg sync.WaitGroup
	wg.Add(1)
	fileDl.OnStart(func(id string) {
		fmt.Println(GetDownloader(id).File.Name, "download started")
		for {
			select {
			case <-exit:
				fmt.Println("\n", fileDl.File.Name, "download finished")
				wg.Done()
			default:
				time.Sleep(time.Second * 1)
				status := fileDl.GetStatus()
				var i = float64(status.Downloaded) / float64(fileDl.File.Size) * 50
				h := strings.Repeat("=", int(i)) + strings.Repeat(" ", 50-int(i))
				fmt.Printf("\r%v/%v [%s] %v byte/s     ", status.Downloaded, fileDl.File.Size, h, status.Speeds)
				os.Stdout.Sync()
			}
		}
	})

	fileDl.OnFinish(func(id string) {
		exit <- true
	})

	fileDl.OnError(func(id string, errCode int, errStr string) {
		log.Fatal(GetDownloader(id).File.Name, errStr)
	})

	fmt.Printf("%+v\n", fileDl)

	fileDl.Start()
	wg.Wait()
}
