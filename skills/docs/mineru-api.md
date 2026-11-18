# MinerU API 速查（本技能用到的子集）

> 官方文档：https://mineru.net/apiManage/docs ；本文件只固化**本技能的调用面**，
> 官方变更时先查官方文档，再改 `config-set mineru.*` 或代码默认值。

## 端点（基址默认 `https://mineru.net/api/v4`，config 可覆盖）

| 方法 | 路径 | 用途 |
|---|---|---|
| POST | `/file-urls/batch` | 建批任务+取预签名上传 URL（body: files[{name,data_id}]/model_version/is_ocr/enable_formula/enable_table/language） |
| PUT | <预签名 URL> | 直传 PDF（分块，1MiB） |
| GET | `/extract-results/batch/{batch_id}` | 轮询（state: pending/running/done/failed + extract_progress + full_zip_url） |
| GET | full_zip_url | 下载结果 zip（含 auto/ 布局产物） |

## 认证与要点

- `Authorization: Bearer <MINERU_TOKEN>`（token 在 profile.env / 环境变量，不落盘别处）；
- data_id 限 128 字符（超长自动截断+哈希后缀，mineruapi.dataIDForUpload）；
- 断点续跑：batch_id 落盘 `.mineru_task.json`，重跑续等/续下不重传；
- 常见失败：DNS/网络不通（换网络或私有端点 MINERU_API_BASE_URL）、token 失效（重新申请）、限流（退避重试，已实现 3 次）。
