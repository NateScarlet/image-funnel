package util

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func AtomicSave(name string, write func(file *os.File) (err error), options ...AtomicOption) (err error) {
	var opts = newAtomicSaveOptions(options...)
	if opts.tmpSuffix == "" {
		return fmt.Errorf("empty tmpSuffix")
	}
	var ok bool
	var tempName string
	defer func() {
		if tempName != "" && !ok {
			// 操作失败，删除临时文件
			err = errors.Join(err, ignoreOSNotExist(os.Remove(tempName)))
		}
	}()

	// 写入临时文件
	err = func() (err error) {
		var dir = filepath.Dir(name)
		var tmpPattern = filepath.Base(name)
		if index := strings.Index(tmpPattern, "."); index >= 0 {
			// only keep first part
			tmpPattern = tmpPattern[:index]
		}
		if len([]rune(tmpPattern)) > 16 {
			// truncate filename if too long
			tmpPattern = string([]rune(tmpPattern)[:16])
		}
		tmpPattern += "~*" + opts.tmpSuffix
		f, err := os.CreateTemp(dir, tmpPattern)
		if err != nil {
			return
		}
		defer f.Close()
		tempName = f.Name()
		err = write(f)
		if err != nil {
			return
		}
		return
	}()
	if err != nil {
		return err
	}

	// 创建备份，防止非原子重命名导致数据永久丢失
	var backupName = name + opts.backupSuffix
	if backupName != name {
		err = os.Link(name, backupName)
		if errors.Is(err, os.ErrNotExist) {
			// 不存在源文件，不需要备份
			err = nil
		} else if errors.Is(err, os.ErrExist) {
			// 已有备份，说明之前的操作可能没成功，保留之前的备份
			err = nil
		} else if err != nil {
			return err
		}
		// 仅在成功后删除备份
		defer func() {
			if ok {
				err = errors.Join(err, ignoreOSNotExist(os.Remove(backupName)))
			}
		}()
	}
	if opts.forceRenameErrorForTest != nil {
		// 单元测试中需要强制出错
		return opts.forceRenameErrorForTest
	}
	// 重命名
	err = os.Rename(tempName, name)
	if err != nil {
		return err
	}
	ok = true
	return nil
}

func ignoreOSNotExist(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func newAtomicSaveOptions(options ...AtomicOption) *AtomicSaveOptions {
	var opts = new(AtomicSaveOptions)
	opts.tmpSuffix = ".tmp"
	opts.backupSuffix = "~"
	for _, i := range options {
		i(opts)
	}
	return opts
}

type AtomicSaveOptions struct {
	tmpSuffix               string
	backupSuffix            string
	forceRenameErrorForTest error
}

type AtomicOption func(opts *AtomicSaveOptions)

func AtomicSaveWithBackupSuffix(v string) AtomicOption {
	return func(opts *AtomicSaveOptions) {
		opts.backupSuffix = v
	}
}
