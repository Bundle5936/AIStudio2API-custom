import os, glob, json, re, shutil, subprocess, time

SRC_DIR = '/uixb/aistudio2api/auth'
OMNI_DIR = '/uixb/aistudio-omni'
AUTH_DIR = os.path.join(OMNI_DIR, 'auth')
LEASES_DIR = os.path.join(AUTH_DIR, '.leases')

print('[1] Copying credentials from aistudio-to-api to aistudio-omni...')
synced_count = 0
for src_file in sorted(glob.glob(os.path.join(SRC_DIR, 'auth-*.json'))):
    try:
        with open(src_file, 'r', encoding='utf-8') as f:
            data = json.load(f)
        
        email = data.get('accountName')
        if not email:
            m = re.search(r'[a-zA-Z0-9._%+-]+@gmail\.com', json.dumps(data))
            email = m.group(0) if m else None
            
        if not email:
            print(f'  [!] Skipping {os.path.basename(src_file)}: could not determine email')
            continue
            
        target_account_dir = os.path.join(AUTH_DIR, email)
        os.makedirs(target_account_dir, exist_ok=True)
        
        target_storage_file = os.path.join(target_account_dir, 'storage-state.json')
        
        # Save backup
        if os.path.exists(target_storage_file):
            shutil.copy2(target_storage_file, target_storage_file + '.bak')
            
        # Write storage-state.json in standard format
        clean_state = {
            'accountName': email,
            'cookies': data.get('cookies', []),
            'origins': data.get('origins', [])
        }
        with open(target_storage_file, 'w', encoding='utf-8') as f:
            json.dump(clean_state, f, indent=2, ensure_ascii=False)
            
        # Ensure account.json exists
        account_json_path = os.path.join(target_account_dir, 'account.json')
        if not os.path.exists(account_json_path):
            with open(account_json_path, 'w', encoding='utf-8') as f:
                json.dump({
                    'label': email,
                    'enabled': True,
                    'proxy': '',
                    'locale': 'en-US',
                    'timezone': 'Etc/UTC'
                }, f, indent=2)
                
        # Clean cooldowns in runtime-state.json
        runtime_state_path = os.path.join(target_account_dir, 'runtime-state.json')
        if os.path.exists(runtime_state_path):
            try:
                with open(runtime_state_path, 'r', encoding='utf-8') as f:
                    rdata = json.load(f)
                if 'cooldowns' in rdata:
                    del rdata['cooldowns']
                    with open(runtime_state_path, 'w', encoding='utf-8') as f:
                        json.dump(rdata, f, indent=2, ensure_ascii=False)
                    print(f'  [-] Cleared cooldowns for {email}')
            except Exception as e:
                print(f'  [!] Error cleaning runtime-state for {email}: {e}')
                
        synced_count += 1
        print(f'  [+] Synced {os.path.basename(src_file)} -> {email} ({len(clean_state["cookies"])} cookies)')
    except Exception as e:
        print(f'  [!] Failed to process {src_file}: {e}')

print(f'[+] Total synced: {synced_count} accounts.')

print('[2] Resetting all account locks in .leases...')
if os.path.exists(LEASES_DIR):
    for lock_file in glob.glob(os.path.join(LEASES_DIR, '*.lock')):
        try:
            os.remove(lock_file)
            print(f'  [-] Removed lock: {os.path.basename(lock_file)}')
        except Exception as e:
            print(f'  [!] Failed to remove {lock_file}: {e}')

print('[3] Reset complete.')
