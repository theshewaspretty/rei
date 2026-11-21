// Package web 은 관제 화면의 정적 파일을 서버 실행 파일에 넣는다.
package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var files embed.FS

// Static 은 index.html 이 루트에 오도록 static 폴더를 돌려준다.
func Static() fs.FS {
	sub, err := fs.Sub(files, "static")
	if err != nil {
		panic(err)
	}
	return sub
}
