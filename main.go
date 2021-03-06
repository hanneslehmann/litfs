package main

import (
	"encoding/json"
	"fmt"
	"github.com/anaskhan96/litfs/disklib"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/anaskhan96/litfs/filesys"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

var inodeCount uint64

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Provide directory for mounting")
		os.Exit(1)
	}

	mountpoint := os.Args[1]

	conn, err := fuse.Mount(mountpoint)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	server := fs.New(conn, nil)

	var fsys *filesys.FS
	if _, err := os.Stat("disklib/sda"); err == nil {
		f, _ := disklib.OpenDisk("disklib/sda", disklib.DISKSIZE)
		defer f.Close()
		metadataBytes := make([]byte, disklib.BLKSIZE)
		disklib.ReadBlock(f, 0, &metadataBytes)
		metablock := make([]byte, disklib.BLKSIZE)
		disklib.ReadBlock(f, 1, &metablock)
		disklib.DiskToMeta(metablock)
		metadataMap := make(map[string]interface{})
		json.Unmarshal(metadataBytes, &metadataMap)
		rootDir, _ := metadataMap["RootDir"].(map[string]interface{})
		root := setupDir(rootDir)
		fsys = &filesys.FS{&root}
	} else {
		fsys = &filesys.FS{&filesys.Dir{}}
	}
	if inodeCount != 0 {
		inodeCount--
	}
	filesys.InitInode(inodeCount)

	log.Println("About to serve fs")
	//go cleanup(fsys)
	if err := server.Serve(fsys); err != nil {
		log.Panicln(err)
	}

	<-conn.Ready
	if err := conn.MountError; err != nil {
		log.Panicln(err)
	}
}

func setupDir(m map[string]interface{}) filesys.Dir {
	var dir filesys.Dir
	for key, value := range m {
		if key == "Inode" {
			inode, _ := value.(float64)
			dir.Inode = uint64(inode)
			inodeCount++
		} else if key == "Name" {
			dir.Name, _ = value.(string)
		} else if key == "Files" {
			var files []*filesys.File
			allFiles, ok := value.([]interface{})
			if !ok {
				dir.Files = nil
				continue
			}
			for _, i := range allFiles {
				val, _ := i.(map[string]interface{})
				file := setupFile(val)
				files = append(files, &file)
			}
			dir.Files = &files
		} else if key == "Directories" {
			var dirs []*filesys.Dir
			allDirs, ok := value.([]interface{})
			if !ok {
				dir.Directories = nil
				continue
			}
			for _, i := range allDirs {
				val, _ := i.(map[string]interface{})
				dirToAppend := setupDir(val)
				dirs = append(dirs, &dirToAppend)
			}
			dir.Directories = &dirs
		}
	}
	return dir
}

func setupFile(m map[string]interface{}) filesys.File {
	var file filesys.File
	for key, value := range m {
		if key == "Inode" {
			inode, _ := value.(float64)
			file.Inode = uint64(inode)
			inodeCount++
		} else if key == "Name" {
			file.Name, _ = value.(string)
			/*} else if key == "Data" {
			data, _ := value.(string)
			file.Data, _ = base64.StdEncoding.DecodeString(data)*/
		} else if key == "Blocks" {
			var blocks []int
			allBlocks, ok := value.([]interface{})
			if !ok {
				file.Blocks = nil
				continue
			}
			for _, i := range allBlocks {
				val, _ := i.(float64)
				blocks = append(blocks, int(val))
			}
			file.Blocks = blocks
		} else if key == "Size" {
			size, _ := value.(float64)
			file.Size = uint64(size)
		}
	}
	return file
}

/* Not being used; replaced by Destroy() in filesys/fs.go */
func cleanup(fsys *filesys.FS) {
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		done <- true
	}()

	<-done
	metadata, err := json.Marshal(&fsys)
	if err != nil {
		log.Println(err)
	}
	f, err := disklib.OpenDisk("disklib/sda", disklib.DISKSIZE)
	defer f.Close()
	disklib.WriteBlock(f, 0, metadata)
	disklib.MetaToDisk(f)

	// Unmounting the directory
	err = fuse.Unmount("data")
	if err != nil {
		log.Println(err)
	}
}
