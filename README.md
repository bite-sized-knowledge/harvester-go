# Harvester-Go

Bite 플랫폼의 기술 블로그 수집기. 매 시간 자동으로 블로그를 크롤링하여 기사를 수집합니다.

## 크롤 타입

| 타입 | 설명 |
|------|------|
| RSS | RSS/Atom 피드 파싱 |
| DEFAULT | 정적 HTML + CSS 셀렉터 기반 스크래핑 |
| MEDIUM | Medium 피드 변환 |
| JINA | Jina AI Reader API를 통한 JS 렌더링 페이지 수집 |
| DEVOCEAN | SK 데보션 전용 (onclick ID 추출) |

## 수집 블로그 목록

### 해외

| 블로그 | URL | 크롤 타입 |
|--------|-----|-----------|
| Airbnb | https://medium.com/airbnb-engineering | MEDIUM |
| AWS Architecture | https://aws.amazon.com/blogs/architecture/ | RSS |
| AWS Compute | https://aws.amazon.com/blogs/compute/ | RSS |
| AWS Containers | https://aws.amazon.com/blogs/containers/ | RSS |
| AWS Database | https://aws.amazon.com/blogs/database/ | RSS |
| AWS DevOps | https://aws.amazon.com/blogs/devops/ | RSS |
| AWS ML | https://aws.amazon.com/blogs/machine-learning/ | RSS |
| AWS Open Source | https://aws.amazon.com/blogs/opensource/ | RSS |
| Claude | https://claude.com/blog | DEFAULT |
| Cloudflare | https://blog.cloudflare.com/ko-kr/ | DEFAULT |
| Delivery Hero | https://deliveryhero.jobs/blog/ | DEFAULT |
| Docker | https://www.docker.com/blog | DEFAULT |
| Dropbox | https://dropbox.tech/all-stories | DEFAULT |
| eBay | https://innovation.ebayinc.com/stories/ | DEFAULT |
| GitHub | https://github.blog/latest | DEFAULT |
| Google | https://developers.googleblog.com/ | DEFAULT |
| Grab | https://engineering.grab.com | RSS |
| Grafana | https://grafana.com/categories/engineering/ | DEFAULT |
| LinkedIn | https://www.linkedin.com/blog/engineering | DEFAULT |
| McDonald's | https://medium.com/mcdonalds-technical-blog/ | MEDIUM |
| Meta | https://engineering.fb.com/ | DEFAULT |
| Microsoft Engineering | https://devblogs.microsoft.com/engineering-at-microsoft/ | RSS |
| Netflix | https://netflixtechblog.com/ | MEDIUM |
| NVIDIA | https://developer.nvidia.com/blog/ | DEFAULT |
| OpenAI | https://developers.openai.com/blog | DEFAULT |
| ParadeDB | https://www.paradedb.com/blog | DEFAULT |
| Pinterest | https://medium.com/pinterest-engineering/ | MEDIUM |
| Slack | https://slack.engineering/ | DEFAULT |
| Spotify | https://engineering.atspotify.com/ | DEFAULT |
| Spring | https://spring.io/blog | DEFAULT |
| trivago | https://tech.trivago.com/posts | DEFAULT |

### 국내

| 블로그 | URL | 크롤 타입 |
|--------|-----|-----------|
| 강남언니 | https://blog.gangnamunni.com/blog/tech/ | DEFAULT |
| 네이버 D2 | https://d2.naver.com | RSS |
| 네이버 플레이스 | https://medium.com/naver-place-dev/ | MEDIUM |
| 네이버페이 | https://medium.com/naverfinancial/ | MEDIUM |
| 당근 | https://medium.com/daangn | RSS |
| 데보션 (SK) | https://devocean.sk.com/blog/index.do | DEVOCEAN |
| 데브시스터즈 | https://tech.devsisters.com/ | DEFAULT |
| 롯데 ON | https://techblog.lotteon.com/ | MEDIUM |
| 리디 | https://ridicorp.com/story-category/tech-blog | RSS |
| 무신사 | https://medium.com/musinsa-tech/ | MEDIUM |
| 뱅크샐러드 | https://blog.banksalad.com/tech/ | DEFAULT |
| 삼성 | https://techblog.samsung.com/ | DEFAULT |
| 삼쩜삼 | https://blog.3o3.co.kr/tag/tech/ | DEFAULT |
| 쏘카 | https://tech.socarcorp.kr | RSS |
| 스포카 | https://spoqa.github.io | RSS |
| 오늘의집 | https://www.bucketplace.com/culture/Tech | DEFAULT |
| 올리브영 | https://oliveyoung.tech/ | DEFAULT |
| 요기요 | https://techblog.yogiyo.co.kr/ | MEDIUM |
| 우아한형제들 | https://techblog.woowahan.com | RSS |
| 왓챠 | https://medium.com/watcha/ | MEDIUM |
| 인포그랩 | https://insight.infograb.net/blog | RSS |
| 인프랩 | https://tech.inflab.com | RSS |
| 여기어때 | https://techblog.gccompany.co.kr/ | MEDIUM |
| 카카오 | https://tech.kakao.com/blog | RSS |
| 카카오모빌리티 | https://developers.kakaomobility.com/techblogs | JINA |
| 카카오스타일 | https://devblog.kakaostyle.com/ko | JINA |
| 카카오페이 | https://tech.kakaopay.com/ | RSS |
| 컬리 | https://helloworld.kurly.com/ | DEFAULT |
| 컴투스온 | https://on.com2us.com/ | DEFAULT |
| 쿠팡 | https://medium.com/coupang-engineering/ | MEDIUM |
| 토스 | https://toss.tech | RSS |
| kt cloud | https://tech.ktcloud.com/ | DEFAULT |
| CLOVA | https://clova.ai/tech-blog | DEFAULT |
| KREAM | https://medium.com/kream-기술-블로그/ | MEDIUM |
| LY Corporation | https://techblog.lycorp.co.jp | RSS |
| NDS Cloud | https://tech.cloud.nongshim.co.kr/Post/ | DEFAULT |
| NHN Cloud | https://meetup.nhncloud.com/ | RSS |
| NOL | https://medium.com/@nol.tech | MEDIUM |
| SK플래닛 | https://techtopic.skplanet.com/ | DEFAULT |
| SSG | https://medium.com/ssgtech/ | MEDIUM |
| 하이퍼커넥트 | https://hyperconnect.github.io/ | DEFAULT |
| 한글과컴퓨터 | https://tech.hancom.com/blog | DEFAULT |
