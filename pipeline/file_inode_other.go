//go:build !unix

package main

import "os"

func fileInode(os.FileInfo) uint64 { return 0 }
