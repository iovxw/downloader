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
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"
)

var (
	// 最大线程数量
	MaxThread = 5
	// 缓冲区大小
	CacheSize = 1024
)

// 创建新的文件下载
//
// 如果想定义其他属性，应手动创建 *FileDl
func NewFileDl(url string, file *os.File) (*FileDl, error) {
	// 获取文件信息
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	size := resp.ContentLength

	f := &FileDl{
		Url:  url,
		Size: size,
		File: file,
	}

	return f, nil
}

type FileDl struct {
	Url  string   // 下载地址
	Size int64    // 文件大小
	File *os.File // 要写入的文件

	BlockList []Block // 用于记录未下载的文件块起始位置

	onStart  func()
	onPause  func()
	onResume func()
	onDelete func()
	onFinish func()
	onError  func(errCode int, errStr string)

	paused bool
	status Status
}

// 开始下载
func (f *FileDl) Start() {
	go func() {
		if f.Size <= 0 {
			f.BlockList = append(f.BlockList, Block{0, -1})
		} else {
			blockSize := f.Size / int64(MaxThread)
			var begin int64
			// 数据平均分配给各个线程
			for i := 0; i < MaxThread; i++ {
				var end = (int64(i) + 1) * blockSize
				f.BlockList = append(f.BlockList, Block{begin, end})
				begin = end + 1
			}
			// 将余出数据分配给最后一个线程
			f.BlockList[MaxThread-1].End += f.Size - f.BlockList[MaxThread-1].End
		}

		// 开始下载
		err := f.download()
		if err != nil {
			f.touchOnError(0, err.Error())
			return
		}
	}()

	f.touch(f.onStart)
}

func (f *FileDl) download() error {
	f.startGetSpeeds()

	ok := make(chan bool, MaxThread)
	for i := range f.BlockList {
		go func(id int) {
			defer func() {
				ok <- true
			}()

			err := f.downloadBlock(id)
			if err != nil {
				// TODO: 自动重新下载块
			}
		}(i)
	}

	for i := 0; i < MaxThread; i++ {
		<-ok
	}
	// 检查是否为暂停
	if !f.paused {
		f.paused = true
		err := os.Remove(f.File.Name() + ".dl")
		if err != nil {
			return err
		}
		f.touch(f.onFinish)
	}
	return nil
}

// 文件块下载器
// 根据线程ID获取下载块的起始位置
func (f *FileDl) downloadBlock(id int) error {
	request, err := http.NewRequest("GET", f.Url, nil)
	if err != nil {
		return err
	}
	begin := f.BlockList[id].Begin
	end := f.BlockList[id].End
	if end != -1 {
		request.Header.Set(
			"Range",
			"bytes="+strconv.FormatInt(begin, 10)+"-"+strconv.FormatInt(end, 10),
		)
	}

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var buf = make([]byte, CacheSize)
	for {
		switch {
		case f.paused == true:
			return nil
		default:
			n, e := resp.Body.Read(buf)

			func() {
				// 将缓冲数据写入硬盘
				f.File.WriteAt(buf[:n], f.BlockList[id].Begin)
			}()

			// 保存块的下载信息。用于断点续传
			byt, err := json.Marshal(f.BlockList)
			if err != nil {
				return err
			}
			err = ioutil.WriteFile(f.File.Name()+".dl", byt, 0644)
			if err != nil {
				return err
			}

			f.status.Downloaded += int64(len(buf[:n]))
			f.BlockList[id].Begin += int64(len(buf[:n]))

			if e != nil {
				if e == io.EOF {
					return nil
				}
				return e
			}
		}
	}

	return nil
}

func (f *FileDl) startGetSpeeds() {
	go func() {
		var old = f.status.Downloaded
		for {
			if f.paused {
				f.status.Speeds = 0
				return
			} else {
				time.Sleep(time.Second * 1)
				f.status.Speeds = f.status.Downloaded - old
				old = f.status.Downloaded
			}
		}
	}()
}

// 获取下载统计信息
func (f FileDl) GetStatus() Status {
	return f.status
}

// 暂停下载
func (f *FileDl) Pause() {
	f.paused = true
	f.touch(f.onPause)
}

// 继续下载
func (f *FileDl) Resume() {
	f.paused = false
	go func() {
		if f.BlockList == nil {
			byt, err := ioutil.ReadFile(f.File.Name() + ".dl")
			if err != nil {
				f.touchOnError(0, err.Error())
				return
			}

			err = json.Unmarshal(byt, f.BlockList)
			if err != nil {
				f.touchOnError(0, err.Error())
				return
			}
		}

		err := f.download()
		if err != nil {
			f.touchOnError(0, err.Error())
			return
		}
	}()
	f.touch(f.onResume)
}

// 任务开始时触发的事件
func (f *FileDl) OnStart(fn func()) {
	f.onStart = fn
}

// 任务暂停时触发的事件
func (f *FileDl) OnPause(fn func()) {
	f.onPause = fn
}

// 任务继续时触发的事件
func (f *FileDl) OnResume(fn func()) {
	f.onResume = fn
}

// 任务完成时触发的事件
func (f *FileDl) OnFinish(fn func()) {
	f.onFinish = fn
}

// 任务出错时触发的事件
//
// errCode为错误码，errStr为错误描述
func (f *FileDl) OnError(fn func(errCode int, errStr string)) {
	f.onError = fn
}

// 用于触发事件
func (f FileDl) touch(fn func()) {
	if fn != nil {
		go fn()
	}
}

// 触发Error事件
func (f FileDl) touchOnError(errCode int, errStr string) {
	if f.onError != nil {
		go f.onError(errCode, errStr)
	}
}

type Status struct {
	Downloaded int64
	Speeds     int64
}

type Block struct {
	Begin int64 `json:"begin"`
	End   int64 `json:"end"`
}
