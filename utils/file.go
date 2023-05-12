package utils

import (
	"bufio"
	"io"
	"io/ioutil"
	"os"
)

func CreateFile(filename string) error {
	newFile, err := os.Create(filename)
	defer newFile.Close()
	return err
}

func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, err
	}
	return false, err
}

func GetFileLines(filename string) (fileLine int64, err error) {
	file, err := os.OpenFile(filename, os.O_RDONLY, 0766)
	if err != nil {
		return fileLine, err
	}
	defer file.Close()

	fileLine = 1
	r := bufio.NewReader(file)
	for {
		_, err := r.ReadString('\n')
		if err != nil || err == io.EOF {
			break
		}
		fileLine += 1
	}
	return fileLine, nil
}

// 如果路径文件存在返回 true，
// 如果不存在，尝试创建它
func PathIsValid(fp string) bool {
	if _, err := os.Stat(fp); err == nil {
		return true
	}

	var d []byte
	if err := ioutil.WriteFile(fp, d, 0644); err == nil {
		os.Remove(fp)
		return true
	}

	return false
}
