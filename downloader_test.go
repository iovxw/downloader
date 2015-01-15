package downloader

import (
	"fmt"
	"log"
	"testing"
)

func Test_NewFile(t *testing.T) {
	file, err := NewFile("", "http://packages.linuxdeepin.com/ubuntu/dists/devel/main/binary-amd64/Packages.bz2", -1, "/tmp", "")
	if err != nil {
		log.Println(err)
	}

	fmt.Printf("%#v\n", file)
}
