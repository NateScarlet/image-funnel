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
		// 加 `.` 前缀，避免 Obsidian 实时监控把未写完的临时文件当作快照读取造成写入冲突
		tmpPattern = "." + tmpPattern + "~*" + opts.tmpSuffix
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
		if opts.perm != 0 {
			// 内容写完后再设定权限：临时文件默认以 0600 创建，
			// 落位前显式 chmod 才能让目标文件保持调用方期望的可读性，且不会阻塞写入本身
			err = f.Chmod(opts.perm)
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
	// 重命名。目标文件可能被外部程序短暂持有（目录监视器对新文件做 stat、杀毒扫描刚写出的
	// 字节），故经 RenameWithRetry 跨过瞬时占用窗口；非瞬时错误仍立即上抛。
	err = RenameWithRetry(tempName, name)
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
	perm                    os.FileMode
	forceRenameErrorForTest error
}

type AtomicOption func(opts *AtomicSaveOptions)

func AtomicSaveWithBackupSuffix(v string) AtomicOption {
	return func(opts *AtomicSaveOptions) {
		opts.backupSuffix = v
	}
}

// AtomicSaveWithPerm 指定落位文件的权限。
// 不指定（零值 0）时沿用临时文件自身的默认权限，不做额外 chmod。
func AtomicSaveWithPerm(perm os.FileMode) AtomicOption {
	return func(opts *AtomicSaveOptions) {
		opts.perm = perm
	}
}
