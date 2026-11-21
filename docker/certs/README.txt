사내망의 TLS 검사 장비 때문에 `docker compose build` 가
"x509: certificate signed by unknown authority" 로 실패하면,
그 장비의 루트 CA 인증서(PEM 형식)를 확장자 .crt 로 이 폴더에 넣고 다시 빌드하세요.

빌드 단계에서 Go 모듈을 받을 때만 쓰이고, 최종 실행 이미지에는 들어가지 않습니다.
이 폴더의 *.crt 는 .gitignore 로 커밋에서 제외됩니다.
