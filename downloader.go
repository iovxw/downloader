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
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"
)

var (
	// 用于获取文件名
	getName = regexp.MustCompile(`^.{0,}?([^/&=]+)$`)
	// 随机数生成器，生成ID用。放在全局以不用多次生成生成器
	r = rand.New(rand.NewSource(time.Now().UnixNano()))

	// 文件名获取错误，用于精细错误处理
	GetFileNameErr = errors.New("can not get file name")

	downloaders = make(map[string]*FileDl)
)

// 获取一个文件的下载信息
func GetDownloader(id string) *FileDl {
	return downloaders[id]
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

	downloaders[f.ID] = f

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
}

type FileInfo struct {
	Name string // 想要保存的文件名
	Url  string // 下载地址
	Size int64  // 文件大小
}

func (f FileDl) Start() {
	go func() {
		fInfo, err := os.Stat(f.StoreDir)
		if err != nil {
			err = os.MkdirAll(f.StoreDir, 0700)
			if err != nil {
				f.touchOnError(0, err.Error())
				return
			}
		}
		if !fInfo.IsDir() {
			f.touchOnError(0, f.StoreDir+" is a file")
			return
		}

		file, err := os.Create(f.StoreDir + string(os.PathSeparator) + f.File.Name)
		if err != nil {
			f.touchOnError(0, err.Error())
			return
		}

		resp, err := http.Get(f.File.Url)
		if err != nil {
			f.touchOnError(0, err.Error())
			return
		}
		defer resp.Body.Close()

		buf, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			f.touchOnError(0, err.Error())
			return
		}

		_, err = io.Copy(file, bytes.NewReader(buf))
		if err != nil {
			f.touchOnError(0, err.Error())
			return
		}

		f.touch(f.onFinish)
	}()

	f.touch(f.onStart)
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
