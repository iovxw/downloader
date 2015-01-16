/*
 Copyright 2015 Bluek404

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package downloader

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"
)

var (
	// 最大线程数量
	MaxThread = 5
	// 缓冲区大小
	CacheSize = 1024

	// 文件名获取错误，用于精细错误处理
	GetFileNameErr = errors.New("can not get file name")

	// 用于获取文件名
	getName        = regexp.MustCompile(`^.{0,}?([^/&=]+)$`)
	downloaderList = make(map[string]*FileDl)
	// 随机数生成器，生成ID用。放在全局以不用多次生成生成器
	r = rand.New(rand.NewSource(time.Now().UnixNano()))
)

// 获取一个文件的下载信息
func GetDownloader(id string) *FileDl {
	return downloaderList[id]
}

// 创建新的文件下载
//
// name, size, id 为可选参数。
func NewFileDl(name string, url string, size int64, storeDir string, id string) (*FileDl, error) {
	// 获取文件信息
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if name == "" {
		buf := getName.FindStringSubmatch(url)
		if len(buf) != 0 {
			name = buf[1]
		} else {
			return nil, GetFileNameErr
		}
	}

	if size <= 0 {
		size = resp.ContentLength
	}

	if id == "" {
		id = newID()
	}

	f := &FileDl{
		File: FileInfo{
			Name: name,
			Url:  url,
			Size: size,
		},
		ID:       id,
		StoreDir: storeDir,
	}

	downloaderList[f.ID] = f

	return f, nil
}

func newID() string { return strconv.FormatInt(r.Int63(), 36) }

type FileDl struct {
	File     FileInfo
	ID       string // 任务ID
	StoreDir string // 保存到的目录

	onStart  func(id string)
	onPause  func(id string)
	onResume func(id string)
	onDelete func(id string)
	onFinish func(id string)
	onError  func(id string, errCode int, errStr string)

	paused    bool
	blockList []block
}

type FileInfo struct {
	Name string // 想要保存的文件名
	Url  string // 下载地址
	Size int64  // 文件大小
	f    *os.File
}

// 开始下载
func (f *FileDl) Start() {
	go func() {
		err := f.openFile(true)
		if err != nil {
			f.touchOnError(0, err.Error())
		}
		defer f.File.f.Close()

		if f.File.Size <= 0 {
			f.blockList = append(f.blockList, block{0, -1})
		} else {
			blockSize := f.File.Size / int64(MaxThread)
			// 数据平均分配给各个线程
			for i := 0; i < MaxThread; i++ {
				f.blockList = append(f.blockList, block{int64(i) * blockSize, (int64(i) + 1) * blockSize})
			}
			// 将余出数据分配给最后一个线程
			f.blockList[MaxThread-1].end += f.File.Size - f.blockList[MaxThread-1].end
		}

		var wg sync.WaitGroup
		wg.Add(MaxThread)
		for i := range f.blockList {
			// TODO: 断点续传支持
			go func(i uint) {
				defer wg.Done()
				err := f.downloadBlock(i)
				if err != nil {

				}
			}(uint(i))
		}
		wg.Wait()

		f.touch(f.onFinish)
	}()

	f.touch(f.onStart)
}

func (f *FileDl) openFile(rewrite bool) error {
	dirInfo, err := os.Stat(f.StoreDir)
	if err != nil {
		err = os.MkdirAll(f.StoreDir, 0644)
		if err != nil {
			return err
		}
	}
	if !dirInfo.IsDir() {
		return errors.New(f.StoreDir + " is a file")
	}

	var file *os.File
	filePath := f.StoreDir + string(os.PathSeparator) + f.File.Name

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		// 文件不存在，创建
		file, err = os.Create(filePath)
		if err != nil {
			return err
		}
	}
	if fileInfo.IsDir() {
		return errors.New(filePath + " is a dir")
	}

	// 文件已存在，是否重写
	if rewrite {
		file, err = os.Create(filePath)
		if err != nil {
			return err
		}
	} else {
		file, err = os.Open(filePath)
		if err != nil {
			return err
		}
	}
	f.File.f = file

	return nil
}

func (f FileDl) downloadBlock(thirdNum uint) error {
	request, err := http.NewRequest("GET", f.File.Url, nil)
	if err != nil {
		return err
	}
	if f.blockList[thirdNum].end != -1 {
		request.Header.Set(
			"Range",
			"bytes="+strconv.FormatInt(f.blockList[thirdNum].begin, 10)+"-"+strconv.FormatInt(f.blockList[thirdNum].end, 10),
		)
	}

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var block = make([]byte, CacheSize)
LOOP:
	for i := 1; ; i++ {
		switch {
		case f.paused == true:
			break LOOP
		default:
			n, err := resp.Body.Read(block)
			fmt.Println(len(block), n, f.blockList[thirdNum].begin)
			f.blockList[thirdNum].begin += int64(len(block[:n]))
			fmt.Println(f.blockList[thirdNum].begin)
			f.writeBlock(block, thirdNum)
			if err != nil {
				if err == io.EOF {
					break LOOP
				}
				return err
			}
		}
	}

	return nil
}

func (f *FileDl) writeBlock(block []byte, thirdNum uint) {
	f.File.f.WriteAt(block, f.blockList[thirdNum].begin)
}

func (f FileDl) Pause() {
	f.paused = true
	f.touch(f.onPause)
}

func (f FileDl) Resume() {
	f.paused = false
	f.touch(f.onResume)
}

// 任务开始时触发的事件
func (f *FileDl) OnStart(fn func(id string)) {
	f.onStart = fn
}

// 任务暂停时触发的事件
func (f *FileDl) OnPause(fn func(id string)) {
	f.onPause = fn
}

// 任务继续时触发的事件
func (f *FileDl) OnResume(fn func(id string)) {
	f.onResume = fn
}

// 任务删除时触发的事件
func (f *FileDl) OnDelete(fn func(id string)) {
	f.onDelete = fn
}

// 任务完成时触发的事件
func (f *FileDl) OnFinish(fn func(id string)) {
	f.onFinish = fn
}

// 任务出错时触发的事件
//
// errCode为错误码，errStr为错误描述
func (f *FileDl) OnError(fn func(id string, errCode int, errStr string)) {
	f.onError = fn
}

func (f FileDl) touch(fn func(id string)) {
	if fn != nil {
		fn(f.ID)
	}
}

func (f FileDl) touchOnError(errCode int, errStr string) {
	if f.onError != nil {
		f.onError(f.ID, errCode, errStr)
	}
}

type block struct {
	begin int64
	end   int64
}
