"""Text-to-speech with VieNeu-TTS (Apache 2.0, runs locally) for `import-audio tts`.

import-audio runs this once per chapter with IN (a UTF-8 text file) and OUT (the WAV to write):

    pip install vieneu numpy
    TTS_CMD='python3 scripts/tts_vieneu.py' go run ./cmd/import-audio tts "Chí Phèo"

VIENEU_VOICE picks a preset voice (default "Hải Đăng"); `python3 scripts/tts_vieneu.py --voices`
lists them. Long chapters are read in sentence-sized chunks and joined with a short pause.
"""

import os
import re
import sys

import numpy as np
from vieneu import Vieneu

MAX_CHARS = 250  # keep each inference short; long inputs degrade quality and use more memory
PAUSE_SEC = 0.35


def chunks(text):
    """Split into sentences, then pack sentences into chunks of at most MAX_CHARS."""
    out, cur = [], ""
    for para in re.split(r"\n\s*\n", text):
        for sent in re.split(r"(?<=[.!?…;:»])\s+", para.strip()):
            if not sent:
                continue
            while len(sent) > MAX_CHARS:  # a very long sentence: cut at the last comma or space
                cut = max(sent.rfind(",", 0, MAX_CHARS), sent.rfind(" ", 0, MAX_CHARS))
                cut = cut if cut > 0 else MAX_CHARS
                out.append(sent[: cut + 1].strip())
                sent = sent[cut + 1 :].strip()
            if cur and len(cur) + 1 + len(sent) > MAX_CHARS:
                out.append(cur)
                cur = sent
            else:
                cur = (cur + " " + sent).strip()
        if cur:  # end chunks at paragraph ends so pauses fall in natural places
            out.append(cur)
            cur = ""
    return out


def main():
    tts = Vieneu()
    if "--voices" in sys.argv:
        for label, voice_id in tts.list_preset_voices():
            print(f"{label}\t{voice_id}")
        return

    src, dst = os.environ["IN"], os.environ["OUT"]
    voice = os.environ.get("VIENEU_VOICE", "Hải Đăng")
    with open(src, encoding="utf-8") as f:
        parts = chunks(f.read())
    rate = int(getattr(tts, "sample_rate", 48000))
    pause = np.zeros(int(rate * PAUSE_SEC), dtype=np.float32)

    audio = []
    for i, part in enumerate(parts, 1):
        print(f"  chunk {i}/{len(parts)}", file=sys.stderr, flush=True)
        wav = np.asarray(tts.infer(part, voice=voice), dtype=np.float32).reshape(-1)
        audio += [wav, pause]
    tts.save(np.concatenate(audio) if audio else pause, dst)


if __name__ == "__main__":
    main()
