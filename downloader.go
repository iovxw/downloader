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
	MaxThread = 10
	// 缓冲区大小
	CacheSize = 10240

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

	paused bool
	status Status
}

type FileInfo struct {
	Name string // 想要保存的文件名
	Url  string // 下载地址
	Size int64  // 文件大小

	f         *os.File
	blockList []block // 用于记录未下载的文件块起始位置
}

type Status struct {
	Downloaded int64
	Speeds     int64
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
			f.File.blockList = append(f.File.blockList, block{0, -1})
		} else {
			blockSize := f.File.Size / int64(MaxThread)
			var begin int64
			// 数据平均分配给各个线程
			for i := 0; i < MaxThread; i++ {
				var end = (int64(i) + 1) * blockSize
				f.File.blockList = append(f.File.blockList, block{begin, end})
				begin = end + 1
			}
			// 将余出数据分配给最后一个线程
			f.File.blockList[MaxThread-1].End += f.File.Size - f.File.blockList[MaxThread-1].End
		}

		f.startGetSpeeds()
		var wg sync.WaitGroup
		wg.Add(MaxThread)
		for i := range f.File.blockList {
			// TODO: 断点续传支持
			go func(id int) {
				defer wg.Done()

				err := f.downloadBlock(id)
				if err != nil {

				}
			}(i)
		}
		wg.Wait()
		f.paused = true
		err = os.Remove(f.StoreDir + string(os.PathSeparator) + f.File.Name + ".dl")
		if err != nil {
			f.touchOnError(0, err.Error())
		}
		f.touch(f.onFinish)
	}()

	f.touch(f.onStart)
}

// 用于创建文件
// 如果文件已存在则打开文件
// rewrite为true则直接覆盖文件
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

// 文件块下载器
// 根据线程ID获取下载块的起始位置
func (f *FileDl) downloadBlock(id int) error {
	request, err := http.NewRequest("GET", f.File.Url, nil)
	if err != nil {
		return err
	}
	begin := f.File.blockList[id].Begin
	end := f.File.blockList[id].End
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
			n, err := resp.Body.Read(buf)
			// 将缓冲数据写入硬盘
			func() {
				f.File.f.WriteAt(append(buf[:n], []byte{}...), f.File.blockList[id].Begin)
				byt, err := json.Marshal(f.File.blockList)
				if err != nil {
					println(err.Error())
				}
				// 保存块的下载信息。用于断点续传
				err = ioutil.WriteFile(f.StoreDir+string(os.PathSeparator)+f.File.Name+".dl", byt, 0644)
				if err != nil {
					println(err.Error())
				}

				f.status.Downloaded += int64(len(buf[:n]))
			}()
			f.File.blockList[id].Begin += int64(len(buf[:n]))
			if err != nil {
				if err == io.EOF {
					return nil
				}
				return err
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
				time.Sleep(time.Millisecond * 100)
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
func (f FileDl) Pause() {
	f.paused = true
	f.touch(f.onPause)
}

// 继续下载
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

// 用于触发事件
func (f FileDl) touch(fn func(id string)) {
	if fn != nil {
		fn(f.ID)
	}
}

// 触发Error事件
func (f FileDl) touchOnError(errCode int, errStr string) {
	if f.onError != nil {
		f.onError(f.ID, errCode, errStr)
	}
}

type block struct {
	Begin int64 `json:"begin"`
	End   int64 `json:"end"`
}
