# MetaTube SDK Go

[![Build Status](https://img.shields.io/github/actions/workflow/status/metatube-community/metatube-sdk-go/docker.yml?branch=main&style=flat-square&logo=github-actions)](https://github.com/metatube-community/metatube-sdk-go/actions/workflows/release.yml)
[![Go Linter](https://img.shields.io/github/actions/workflow/status/metatube-community/metatube-sdk-go/linter.yml?style=flat-square&branch=main&logo=githubactions&label=golangci-lint)](https://github.com/metatube-community/metatube-sdk-go/actions/workflows/linter.yml)
[![Require Go Version](https://img.shields.io/badge/go-%3E%3D1.25-30dff3?style=flat-square&logo=go)](https://github.com/metatube-community/metatube-sdk-go/blob/main/go.mod)
[![GitHub License](https://img.shields.io/github/license/metatube-community/metatube-sdk-go?color=e4682a&logo=apache&style=flat-square)](https://github.com/metatube-community/metatube-sdk-go/blob/main/LICENSE)
[![Tag](https://img.shields.io/github/v/tag/metatube-community/metatube-sdk-go?color=%23ff8936&logo=fitbit&style=flat-square)](https://github.com/metatube-community/metatube-sdk-go/tags)

Metadata Tube SDK in Golang.

## Contents

- [MetaTube SDK Go](#metatube-sdk-go)
    - [Contents](#contents)
    - [Features](#features)
    - [Installation](#installation)
    - [DMM 高清封面](#dmm-高清封面)
    - [Credits](#credits)
    - [License](#license)

## Features

- Supported platforms
    - Linux
    - Darwin
    - Windows
    - BSD(s)
- Supported Databases
    - [SQLite](https://gitlab.com/cznic/sqlite)
    - [PostgreSQL](https://github.com/jackc/pgx)
- Image processing
    - Auto cropping
    - Badge support
    - Face detection
    - Image hashing
- RESTful API
- 20+ providers
- Text translation

## Installation

To install this package, you first need [Go](https://golang.org/) installed (**go1.25+ is required**), then you can use
the below Go command to install SDK.

```sh
go get -u github.com/metatube-community/metatube-sdk-go
```

## DMM 高清封面

JavBus、JAV321、FANZA 支持可选配置 `use_dmm_hd_cover`，默认关闭。使用服务命令启动时，可按需为各来源开启：

```sh
export MT_MOVIE_PROVIDER_JAVBUS__USE_DMM_HD_COVER=true
export MT_MOVIE_PROVIDER_JAV321__USE_DMM_HD_COVER=true
export MT_MOVIE_PROVIDER_FANZA__USE_DMM_HD_COVER=true
```

SDK 调用方可通过 `engine.WithMovieProviderConfig` 传入包含同名布尔配置的 `provider.Config`。非法布尔值会返回配置错误。

开启后，仅从两种 DMM CDN 的 `digital/video` 预览图路径提取 CID；没有 CID、多个 CID 冲突或与可识别的原封面 CID 不一致时保持原值。不从发行番号猜测 CID，也不改写 `mono/movie` 等其他类目路径。候选图片位于 `awsimgsrc.dmm.co.jp`，经有界 GET 验证 JPEG 头和尺寸后，同时更新 `CoverURL` 和 `BigCoverURL`，不改变缩略图、预览图和其他元数据。验证失败不影响影片信息获取，并保留原封面。

图片验证沿用对应 provider 的代理和请求超时配置（JAV321 维持现有固定 10 秒策略），禁止重定向，最多读取 128 KiB；图片尺寸信息不在这个范围内时不替换。候选图不保证一定比原图清晰，部分老作品仍只有原分辨率。此配置不解决 FANZA 元数据页面的地区访问限制。

开启配置后，已缓存的影片信息需要重新抓取；客户端已有的海报缓存也可能需要刷新。

## Credits

| Library														                                           | Description																						                                                                    |
|-----------------------------------------------------------------|------------------------------------------------------------------------------------------------------|
| [gocolly/colly](https://github.com/gocolly/colly)			            | Elegant Scraper and Crawler Framework for Golang													                                        |
| [gin-gonic/gin](https://github.com/gin-gonic/gin)			            | Gin is a HTTP web framework written in Go															                                             |
| [gorm.io/gorm](https://gorm.io/)								                        | The fantastic ORM library for Golang																                                                 |
| [esimov/pigo](https://github.com/esimov/pigo)				               | Fast face detection, pupil/eyes localization and facial landmark points detection library in pure Go |
| [robertkrimen/otto](https://github.com/robertkrimen/otto)       | A JavaScript interpreter in Go (golang)                                                              |
| [modernc.org/sqlite](https://gitlab.com/cznic/sqlite)		         | Package sqlite is a CGo-free port of SQLite/SQLite3												                                      |
| [corona10/goimagehash](https://github.com/corona10/goimagehash) | Go Perceptual image hashing package																                                                  |
| [antchfx/xpath](https://github.com/antchfx/xpath)			            | XPath package for Golang, supports HTML, XML, JSON document query									                           |
| [gen2brain/jpegli](https://github.com/gen2brain/jpegli)         | Go encoder/decoder for JPEG based on jpegli                                                          |

## License

[Apache-2.0 License](https://github.com/metatube-community/metatube-sdk-go/blob/main/LICENSE)
