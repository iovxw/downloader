package downloader

import (
	"fmt"
	"log"
	"sync"
	"testing"
)

func Test_NewFile(t *testing.T) {
	fileDl, err := NewFileDl("", "http://packages.linuxdeepin.com/ubuntu/dists/devel/main/binary-amd64/Packages.bz2", -1, "/tmp", "")
	if err != nil {
		log.Println(err)
	}

	fileDl.OnStart(func(id string) {
		fmt.Println(GetDownloader(id).File.Name, "download started")
	})

	var wg sync.WaitGroup
	wg.Add(1)
	fileDl.OnFinish(func(id string) {
		fmt.Println(GetDownloader(id).File.Name, "download finished")
		wg.Done()
	})

	fileDl.OnError(func(id string, errCode int, errStr string) {
		log.Println(GetDownloader(id).File.Name, errStr)
	})

	fmt.Printf("%+v\n", fileDl)

	fileDl.Start()
	wg.Wait()
}
