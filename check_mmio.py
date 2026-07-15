import re, os, glob, sys

# Resolve sdk/docs relative to this script so the check works in any checkout,
# not just the one it was written in. An explicit repository root may be passed
# as the first argument.
repo_root = sys.argv[1] if len(sys.argv) > 1 else os.path.dirname(os.path.abspath(__file__))
docs_path = os.path.join(repo_root, 'sdk', 'docs')
if not os.path.isdir(docs_path):
    sys.exit(f"No docs directory at {docs_path}; pass the repository root as an argument.")
files = glob.glob(os.path.join(docs_path, '**', '*.md'), recursive=True)
if not files:
    sys.exit(f"No .md files under {docs_path}; refusing to report every entry as missing.")

patterns = {
    'IE_VIDEO_CTRL': r'0x000F0000|0xF0000',
    'IE_VIDEO_MODE': r'0x000F0004|0xF0004',
    'IE_VRAM_BASE': r'0x00100000|0x100000',
    'IE_PSG_BASE': r'0x000F0C00|0xF0C00',
    'FLEX': r'0xF0A80',
    'SID2': r'0xF0C40',
    'SID3': r'0xF0D40',
    'SFX': r'0xF2600',
    'PSG': r'0xF0C00',
    'PSG_PLAYER': r'0xF0C10',
    'SN76489': r'0xF0C30',
    'SID': r'0xF0E00',
    'TED_AUDIO': r'0xF0F00',
    'POKEY': r'0xF0D00',
    'AHX': r'0xF0B80',
    'MIDI': r'0xF0BA0',
    'MOD': r'0xF0BC0',
    'WAV': r'0xF0BD8',
    'SAP': r'0xF0D10',
    'TED_PLAYER': r'0xF0F10',
    'MEDIA_LOADER': r'0xF2300',
    'PAULA': r'0xF2260',
    'LIVE_MIDI': r'0xF0BF4',
    'VGA': r'0xF1000',
    'TED_VIDEO': r'0xF0F20',
    'ANTIC': r'0xF2100',
    'ULA': r'0xF2000',
    'ULA_VRAM': r'0xFA000',
    'VOODOO': r'0xF8000',
    'AROS_DOS': r'0xF2220',
    'AROS_AUDIO_DMA': r'0xF2260',
}

found = {k: False for k in patterns}

for f in files:
    try:
        with open(f, 'r') as fh:
            content = fh.read()
            for k, p in patterns.items():
                if re.search(p, content, re.IGNORECASE):
                    found[k] = True
    except:
        pass

missing = [k for k, v in found.items() if not v]
for k in missing:
    print(f"Missing or mismatched: {k}")
if missing:
    sys.exit(f"{len(missing)} of {len(patterns)} MMIO addresses missing or mismatched in {docs_path}")
print(f"Done: all {len(patterns)} MMIO addresses documented")
