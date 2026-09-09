#!/usr/bin/env python3
import os
import sys
import glob
import json
import time
import hashlib
import urllib.request
import urllib.parse
import urllib.error

SRC_DIR = "/uixb/aistudio2api/auth"
OMNI_DIR = "/uixb/aistudio-omni"
AUTH_DIR = os.path.join(OMNI_DIR, "auth")
LEASES_DIR = os.path.join(AUTH_DIR, ".leases")
STATE_FILE = os.path.join(OMNI_DIR, ".sync_state.json")
LOG_FILE = os.path.join(OMNI_DIR, "sync.log")
API_BASE = "http://127.0.0.1:2048"

CHECK_INTERVAL = 30             # 巡检间隔 30 秒（避免高频死循环）
REVIVE_COOLDOWN_BASE = 900      # 无新凭据时的单账号拉活最小冷却 15 分钟（防止连环 401 轰炸）
REVIVE_COOLDOWN_MAX = 3600      # 连续失败后最大冷却 1 小时
MAX_LOG_SIZE = 5 * 1024 * 1024  # 日志超过 5MB 自动截断保留最新行

# 记录账号拉活尝试历史: {email: {"last_try": timestamp, "fail_count": int}}
revive_history = {}

def log(msg):
    ts = time.strftime("%Y-%m-%d %H:%M:%S")
    line = f"[{ts}] {msg}"
    print(line, flush=True)
    try:
        with open(LOG_FILE, "a", encoding="utf-8") as f:
            f.write(line + "\n")
    except Exception:
        pass

def check_log_rotation():
    try:
        if os.path.exists(LOG_FILE) and os.path.getsize(LOG_FILE) > MAX_LOG_SIZE:
            with open(LOG_FILE, "r", encoding="utf-8", errors="ignore") as f:
                lines = f.readlines()
            keep_lines = lines[-2000:]
            with open(LOG_FILE, "w", encoding="utf-8") as f:
                f.writelines(keep_lines)
    except Exception:
        pass

def get_file_hash(path):
    try:
        with open(path, "rb") as f:
            return hashlib.sha256(f.read()).hexdigest()
    except Exception:
        return None

def load_state():
    if os.path.exists(STATE_FILE):
        try:
            with open(STATE_FILE, "r", encoding="utf-8") as f:
                return json.load(f)
        except Exception:
            pass
    return {}

def save_state(state):
    try:
        with open(STATE_FILE, "w", encoding="utf-8") as f:
            json.dump(state, f, indent=2)
    except Exception as e:
        log(f"Error saving state: {e}")

def verify_account(aid):
    """调用 verify API 重新加载凭据并激活账号"""
    try:
        encoded_id = urllib.parse.quote(aid)
        url = f"{API_BASE}/api/accounts/{encoded_id}/verify"
        req = urllib.request.Request(url, data=b"", method="POST")
        with urllib.request.urlopen(req, timeout=30) as resp:
            data = json.loads(resp.read().decode("utf-8"))
            acc = data.get("account", {})
            st = acc.get("state")
            if st == "ready":
                log(f"[Verify] Account {aid} successfully verified and ready.")
                return True
            else:
                msg = acc.get("message", "")
                log(f"[Verify] Account {aid} returned state={st}: {msg}")
                return False
    except Exception as e:
        log(f"[Verify] Failed to verify {aid}: {e}")
        return False

def check_and_revive_unhealthy(changed_accounts):
    """巡检所有账号，严格限频自愈拉活，并对刚更新凭据的账号主动刷新内存 Session"""
    now = time.time()

    # 如果上游凭据有更新，重置对应账号的拉活退避计时器
    for email in changed_accounts:
        if email in revive_history:
            revive_history[email]["fail_count"] = 0
            revive_history[email]["last_try"] = 0

    try:
        req = urllib.request.Request(f"{API_BASE}/api/accounts")
        with urllib.request.urlopen(req, timeout=5) as resp:
            data = json.loads(resp.read().decode("utf-8"))
    except Exception:
        # aistudio-omni 服务如果暂时未就绪，跳过
        return

    accounts = data.get("accounts", [])

    # 场景 1: 上游凭据文件刚刚被写入磁盘，说明有新凭据，主动调用 verify 立即重载内存 Session
    for aid in changed_accounts:
        log(f"[Sync-Reload] Disk credentials updated for {aid}, reloading in-memory session...")
        ok = verify_account(aid)
        revive_history[aid] = {
            "last_try": now,
            "fail_count": 0 if ok else 1
        }

    # 场景 2: 账号处于 auth_required 异常状态，平缓自愈
    # 严格节流：每轮巡检最多只尝试拉活 1 个账号，避免 CPU 并发风暴
    revived_this_round = 0
    for a in accounts:
        aid = a.get("id")
        if not aid or aid in changed_accounts:
            continue
        state = a.get("state")

        if state == "auth_required":
            hist = revive_history.get(aid, {"last_try": 0, "fail_count": 0})
            fail_count = hist.get("fail_count", 0)
            cooldown = min(REVIVE_COOLDOWN_BASE * (2 ** min(fail_count, 3)), REVIVE_COOLDOWN_MAX)

            if now - hist.get("last_try", 0) >= cooldown:
                if revived_this_round >= 1:
                    # 本轮已拉活一个账号，其余账号留待后续轮次，避免瞬间打满 CPU
                    break
                log(f"[Auto-Heal] Detected unhealthy account {aid} (state={state}, fail_count={fail_count}). Attempting gentle revival...")
                ok = verify_account(aid)
                revive_history[aid] = {
                    "last_try": now,
                    "fail_count": 0 if ok else (fail_count + 1)
                }
                revived_this_round += 1

def check_and_sync():
    check_log_rotation()
    state = load_state()
    file_hashes = state.get("hashes", {})
    changed_accounts = []

    src_files = sorted(glob.glob(os.path.join(SRC_DIR, "auth-*.json")))
    if src_files:
        for src_file in src_files:
            fname = os.path.basename(src_file)
            current_hash = get_file_hash(src_file)
            if not current_hash:
                continue

            prev_hash = file_hashes.get(fname)
            if current_hash != prev_hash:
                try:
                    with open(src_file, "r", encoding="utf-8") as f:
                        data = json.load(f)

                    email = data.get("accountName")
                    if not email:
                        import re
                        m = re.search(r"[a-zA-Z0-9._%+-]+@gmail\.com", json.dumps(data))
                        email = m.group(0) if m else None

                    if not email:
                        continue

                    target_account_dir = os.path.join(AUTH_DIR, email)
                    os.makedirs(target_account_dir, exist_ok=True)
                    target_storage_file = os.path.join(target_account_dir, "storage-state.json")

                    clean_state = {
                        "accountName": email,
                        "cookies": data.get("cookies", []),
                        "origins": data.get("origins", [])
                    }
                    with open(target_storage_file, "w", encoding="utf-8") as f:
                        json.dump(clean_state, f, indent=2, ensure_ascii=False)

                    # 清理可能存在的过期锁
                    for pattern in [f"{email}.lock", f"{email}.runtime.lock"]:
                        lock_file = os.path.join(LEASES_DIR, pattern)
                        if os.path.exists(lock_file):
                            try:
                                os.remove(lock_file)
                            except Exception:
                                pass

                    changed_accounts.append(email)
                    file_hashes[fname] = current_hash
                except Exception as e:
                    log(f"Failed to sync {fname}: {e}")

        if changed_accounts:
            acc_str = ", ".join(changed_accounts)
            log(f"Quietly updated disk credentials for: {acc_str} (zero downtime)")
            state["hashes"] = file_hashes
            save_state(state)

    # 执行健康巡检与受控自愈
    check_and_revive_unhealthy(changed_accounts)

def main():
    if "--once" in sys.argv:
        check_and_sync()
        return

    log(f"AIStudio Omni Watchdog daemon started (interval={CHECK_INTERVAL}s, safe auto-heal).")
    while True:
        try:
            check_and_sync()
        except Exception as e:
            log(f"Unexpected error in watch loop: {e}")
        time.sleep(CHECK_INTERVAL)

if __name__ == "__main__":
    main()
