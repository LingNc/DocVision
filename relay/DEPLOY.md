# relay 部署指南（doc-relay.lingnc.top · 1Panel 反代版）

relay = 匿名兜底服务器：用户拒绝 GitHub 登录时，agent 经它用**我们的账号**代发 issue（署名=agent 名）。
架构：**relay 纯 HTTP 监听本地端口 ← 1Panel 反代终结 TLS**。

## ① 服务器上跑 relay（二进制 + systemd）

```bash
# 上传 dvrelay-linux-amd64（在 /mnt/agents/output/ 或镜像 dv-skills-v3/）到服务器，如 /opt/relay/
sudo mkdir -p /opt/relay && sudo mv dvrelay-linux-amd64 /opt/relay/dvrelay && sudo chmod +x /opt/relay/dvrelay
# 配置（非秘密项；token 只走环境变量）
sudo tee /opt/relay/relay.yaml <<'EOF'
listen: "127.0.0.1:8787"   # 只给本机反代看
repo: "LingNc/DocVision"
rate: 5
EOF
# systemd 单元（RELAY_GITHUB_TOKEN=有 repo issue 权限的 PAT，注意引号防 # 截断）
sudo tee /etc/systemd/system/dvrelay.service <<'EOF'
[Unit]
Description=DocVision anonymous relay
After=network.target
[Service]
Environment="RELAY_GITHUB_TOKEN=ghp_你的PAT"
WorkingDirectory=/opt/relay
ExecStart=/opt/relay/dvrelay -config /opt/relay/relay.yaml
Restart=always
[Install]
WantedBy=multi-user.target
EOF
sudo systemctl daemon-reload && sudo systemctl enable --now dvrelay
curl -s http://127.0.0.1:8787/health     # {"ok":true} 即活
```

## ② DNS + 1Panel 反代（终结 HTTPS）

1. DNS：A 记录 `doc-relay.lingnc.top` → 服务器 IP
2. 1Panel：**网站 → 创建网站 → 反向代理**
   - 域名：`doc-relay.lingnc.top`
   - 代理地址：`http://127.0.0.1:8787`
3. 该站点 → **SSL → 申请证书**（Let's Encrypt，HTTP 验证；申请后开"强制 HTTPS"）
4. 验证：`curl https://doc-relay.lingnc.top/health` → `{"ok":true}`

## ③ skills 侧（已内置，无需动）

安装包 `config.example.yaml` 的 `upstream.relay_url` 默认就是 `https://doc-relay.lingnc.top`；
用户使用前 `cp config.example.yaml config.yaml` 即生效。`troubleshoot relay-submit` 自动走它。

## 安全面

- relay 只暴露 POST `/submit`（JSON：title/body/author）+ GET `/health`；rate 限每 IP 5 次/时
- 秘钥纪律：token 只在服务器环境变量，**不进 git/不进包**；relay 绑 127.0.0.1 不直接对外
- 提交体在 relay 侧二次脱敏（token/私钥/绝对路径）后才建 issue
