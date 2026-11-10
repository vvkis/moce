#!/usr/bin/env python3
"""Pack a portable 墨册 folder zip. The zip root is the 墨册/ directory."""
import sys
import zipfile
from pathlib import Path

exe = Path(sys.argv[1])
dest = Path(sys.argv[2])
root = Path(__file__).resolve().parents[1]
readme = root / "packaging" / "README.txt"
license_file = root / "LICENSE"
dest.parent.mkdir(parents=True, exist_ok=True)
with zipfile.ZipFile(dest, "w", compression=zipfile.ZIP_DEFLATED) as zf:
    zf.write(exe, "墨册/墨册.exe")
    zf.write(readme, "墨册/README.txt")
    zf.write(license_file, "墨册/LICENSE")
    for folder in ("data", "cache", "fonts"):
        zf.writestr(f"墨册/{folder}/", "")
