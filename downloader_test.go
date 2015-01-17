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
	file, err := os.Create("/tmp/file")
	if err != nil {
		log.Println(err)
	}
	defer file.Close()

	fileDl, err := NewFileDl("http://packages.linuxdeepin.com/ubuntu/dists/devel/main/binary-amd64/Packages.bz2", file, -1)
	if err != nil {
		log.Println(err)
	}

	var exit = make(chan bool)
	var resume = make(chan bool)
	var pause bool
	var wg sync.WaitGroup
	wg.Add(1)
	fileDl.OnStart(func() {
		fmt.Println("download started")
		format := "\033[2K\r%v/%v [%s] %v byte/s %v"
		for {
			status := fileDl.GetStatus()
			var i = float64(status.Downloaded) / float64(fileDl.Size) * 50
			h := strings.Repeat("=", int(i)) + strings.Repeat(" ", 50-int(i))

			select {
			case <-exit:
				fmt.Printf(format, status.Downloaded, fileDl.Size, h, 0, "[FINISH]")
				fmt.Println("\ndownload finished")
				wg.Done()
			default:
				if !pause {
					time.Sleep(time.Second * 1)
					fmt.Printf(format, status.Downloaded, fileDl.Size, h, status.Speeds, "[DOWNLOADING]")
					os.Stdout.Sync()
				} else {
					fmt.Printf(format, status.Downloaded, fileDl.Size, h, 0, "[PAUSE]")
					os.Stdout.Sync()
					<-resume
					pause = false
				}
			}
		}
	})

	fileDl.OnPause(func() {
		pause = true
	})

	fileDl.OnResume(func() {
		resume <- true
	})

	fileDl.OnFinish(func() {
		exit <- true
	})

	fileDl.OnError(func(errCode int, err error) {
		log.Println(errCode, err)
	})

	fmt.Printf("%+v\n", fileDl)

	fileDl.Start()
	time.Sleep(time.Second * 2)
	fileDl.Pause()
	time.Sleep(time.Second * 3)
	fileDl.Resume()
	wg.Wait()
}
