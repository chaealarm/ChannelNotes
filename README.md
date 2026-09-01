# Channel Notes

Channel Notes는 Discord에서 영감을 받은 그룹·채널·카테고리 구조의 Windows 로컬 메모 애플리케이션입니다. Go와 Wails로 작성되며 별도 서버나 계정 없이 PC에 데이터를 저장합니다.

## 주요 기능

- 그룹 → 채널 → 카테고리 → 메모장 계층 구조
- 제목·목록 명칭 연동과 사용자 지정 명칭
- 시스템 글꼴, 직접 입력 가능한 글자 크기, 테마 대응 텍스트 색상
- JPG, JPEG, PNG, WEBP, GIF 이미지 삽입 및 클립보드·드래그 앤 드롭
- 본문 이미지 크기·정렬 조절과 채널 아이콘 크롭 편집
- 시스템 설정 연동, 어두운 테마, 밝은 테마
- 변경 후 자동저장 및 저장 완료 시각 표시
- 전체 데이터 또는 개별 그룹 ZIP 백업·복원
- 현재 메모 HTML 저장 및 HTML 메모 불러오기
- 프로세스 간 그룹 잠금과 그룹 단위 병합 저장

## 요구 사항

- Windows 10/11
- [Go](https://go.dev/) 1.22 이상
- [Node.js](https://nodejs.org/) 20 이상
- [Wails CLI](https://wails.io/docs/gettingstarted/installation) 2.9 이상
- Microsoft Edge WebView2 Runtime

## 개발 실행

```powershell
cd frontend
npm install
cd ..
wails dev
```

## 프로덕션 빌드

```powershell
wails build -clean -platform windows/amd64
```

완성된 실행 파일은 `build/bin/ChannelNotes.exe`에 생성됩니다. 일반 `go build`는 Wails의 필수 빌드 태그와 Windows 리소스를 적용하지 않으므로 사용하지 않는 것이 좋습니다.

## 데이터 저장 위치

```text
%AppData%\ChannelNotes\data\settings.json
%AppData%\ChannelNotes\data\groups\{groupId}\group.json
%AppData%\ChannelNotes\data\groups\{groupId}\channels\{channelId}\channel.json
%AppData%\ChannelNotes\data\groups\{groupId}\channels\{channelId}\categories\{categoryId}\category.json
%AppData%\ChannelNotes\data\groups\{groupId}\channels\{channelId}\categories\{categoryId}\notes\{noteId}\meta.json
%AppData%\ChannelNotes\data\groups\{groupId}\channels\{channelId}\categories\{categoryId}\notes\{noteId}\content.html
```

메모 본문과 이미지는 각 메모의 `content.html`에 저장되며 선택한 메모만 지연 로딩됩니다. 따라서 데이터가 커져도 하나의 거대한 JSON 파일을 매번 역직렬화하지 않습니다. 그룹 편집 잠금 파일은 `%AppData%\ChannelNotes\locks`에 저장되며, 비정상 종료로 남은 잠금은 프로세스 상태 확인 후 자동 정리됩니다.

자동저장은 입력 후 약 1.2초, 최대 5초 주기, 채널·메모 전환 시 수행됩니다. 여러 창을 실행하면 한 그룹은 한 창에서만 선택할 수 있고, 서로 다른 그룹의 저장 내용은 병합되어 다른 창의 작업을 덮어쓰지 않습니다.

## 백업 권장 사항

중요한 데이터를 이동하거나 대규모로 정리하기 전에는 설정의 **전체 백업**을 권장합니다. 그룹 단위 이동에는 **선택 그룹 백업**과 **그룹 복원**을 사용할 수 있습니다. 복원된 그룹은 기존 그룹과 충돌하지 않도록 새 식별자로 추가됩니다.

생성형 AI를 이용하여 작성된 코드

Special Thanks to 변태아니야(Arcalive)
