package main

import (
	"bytes"
	"io"
	"io/fs"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
)

func get_data(root string) (int, [] string, error){
	var files []string

	err := filepath.WalkDir(root , func(path string, d fs.DirEntry, err error) error {
		if err != nil{
			return err
		}

		if !d.IsDir(){
			files = append(files, path)
		}
		return nil
	})
	
	if err != nil{
		log.Print("Error walking through directory ",err)
	}

	return len(files), files, nil
}

func get_img_data(file_path string) ([]byte , error){
	
	f, err := os.Open(file_path)
	if err != nil{
		log.Fatal("Errror at get img data", err) //handle diff during production
	}

	b := bytes.Buffer{}
	_, err = io.Copy(&b , f)
	if err != nil{
		log.Fatal("Errror at get img data", err) //handle diff during production
	}

	f.Close()

	return b.Bytes(), nil
}

func create_uid() (string){
	uid := rand.IntN(9999)
	uid_str := strconv.Itoa(uid)
	return  uid_str
}
