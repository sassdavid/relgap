# Changelog

## v2.4.8 (2022-07-29)

### Bug fixes

- feat: support application level extensions (#9923)
- feat: support multiple extensions per resource group/kind (#9834)
- fix: extensions is not loading for ConfigMap/Pods (#10010)
- fix: upgrade moment from 2.29.2 to 2.29.3 (#9330)
- fix: skip redirect url validation when it's the base href (#10058) (#10116)
- fix: avoid CVE-2022-28948 (#10093)
- fix: Set HOST_ARCH for yarn build from platform (#10018)

### Other changes

- chore(deps): bump moment from 2.29.3 to 2.29.4 in /ui (#9897)
- docs: add OpenSSH breaking change notes (#10104)
- chore: update parse-url (#10101)
- docs: add api field example in the appset security doc (#10087)
- chore: update redis to 7.0.4 avoid CVE-2022-30065 (#10059)
- docs: add argocd-server grpc metric usage (#10007)
- chore: upgrade Dex to 2.32.0 (#10036) (#10042)
- chore: update redis to avoid CVE-2022-2097 (#10031)
- chore: update haproxy to 2.0.29 for redis-ha (#10045)

## v2.4.7 (2022-07-18)

### Bug fixes

fix: Support files in argocd.argoproj.io/manifest-generate-paths annotation (#9908)
fix: terminal websocket write lock to avoid races (#10011)
fix: updated all a tags to Link tags in app summary (#9777)
fix: e2e test to use func from clusterauth instead creating one with old logic (#9989)
fix: add missing download CLI tool URL response for ppc64le, s390x (#9983)

### Other

chore: upgrade parse-url to avoid SNYK-JS-PARSEURL-2936249 (#9826)
docs: use quotes to emphasize that ConfigMap value is a string (#9995)
