# 강화 설치

기본 이미지도 이미 root가 아닌 사용자로 실행되며, Redis는 생성된 비밀번호로 보호되고 OpenResty는
서명을 검증한 소스로 빌드됩니다.
[`deploy/docker-compose.hardened.yml`](https://github.com/corvus-gateway/corvus/blob/main/deploy/docker-compose.hardened.yml)은
여기서 더 나아가 **읽기 전용 루트 파일 시스템**, **Linux capability 없음**, **권한 상승 금지**를
적용합니다.

먼저 사용자 `10001`용 데이터 디렉터리를 만듭니다.

```bash
mkdir -p data && sudo chown 10001:10001 data
docker compose -f docker-compose.hardened.yml up -d
```

!!! note "큰 파일 업로드"
    이 모드에서는 임시 파일이 256 MB 크기의 메모리 기반 `/tmp`에 저장됩니다. 프록시 호스트가 약
    200 MB보다 큰 업로드를 받는다면 compose 파일에서 `tmpfs` 크기를 늘리세요.
