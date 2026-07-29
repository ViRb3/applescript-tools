#!/usr/bin/env python3
"""Watch a folder and compile AppleScript source files when they change."""

from __future__ import annotations

import argparse
import os
import subprocess
import time
from pathlib import Path

DEFAULT_FOLDER = Path(__file__).resolve().parent / "workdir"


def compile_applescript(source: Path) -> None:
    output = source.with_suffix(".scpt")
    log = source.with_suffix(".log")
    temporary_output = Path(f"{output}.tmp")
    temporary_log = Path(f"{log}.tmp")
    print(
        f"Compiling {source.name} -> {output.name} (log: {log.name})",
        flush=True,
    )

    try:
        output.unlink(missing_ok=True)
        log.unlink(missing_ok=True)
        temporary_output.unlink(missing_ok=True)
        temporary_log.unlink(missing_ok=True)

        with temporary_log.open("w", encoding="utf-8") as log_file:
            result = subprocess.run(
                [
                    "osacompile",
                    "-x",
                    "-o",
                    str(temporary_output),
                    str(source),
                ],
                stdout=log_file,
                stderr=subprocess.STDOUT,
                text=True,
            )
            log_file.flush()
            os.fsync(log_file.fileno())

        if result.returncode == 0:
            with temporary_output.open("rb") as compiled_file:
                os.fsync(compiled_file.fileno())
            os.replace(temporary_output, output)
        else:
            print(
                f"Compilation failed for {source.name} "
                f"(exit code {result.returncode}); see {log.name}",
                flush=True,
            )

        os.replace(temporary_log, log)
    except FileNotFoundError:
        temporary_log.write_text(
            "osacompile was not found. This script must run on macOS.\n",
            encoding="utf-8",
        )
        os.replace(temporary_log, log)
        raise SystemExit(
            "osacompile was not found. This script must run on macOS."
        ) from None
    finally:
        temporary_output.unlink(missing_ok=True)
        temporary_log.unlink(missing_ok=True)


def watch(folder: Path, interval: float) -> None:
    known_files: dict[Path, tuple[int, int]] = {}
    print(f"Watching {folder} for *.applescript files...", flush=True)

    while True:
        current_files: set[Path] = set()

        for source in sorted(folder.glob("*.applescript")):
            if not source.is_file():
                continue

            current_files.add(source)
            stat = source.stat()
            fingerprint = (stat.st_mtime_ns, stat.st_size)

            if known_files.get(source) != fingerprint:
                compile_applescript(source)
                known_files[source] = fingerprint

        for removed_file in known_files.keys() - current_files:
            del known_files[removed_file]

        time.sleep(interval)


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Compile *.applescript files when they are created or modified."
    )
    parser.add_argument(
        "folder",
        nargs="?",
        type=Path,
        default=DEFAULT_FOLDER,
        help="folder to watch (default: ./workdir beside this script)",
    )
    parser.add_argument(
        "--interval",
        type=float,
        default=0.1,
        help="polling interval in seconds (default: 0.1)",
    )
    args = parser.parse_args()

    folder = args.folder.expanduser().resolve()
    if not folder.is_dir():
        parser.error(f"not a directory: {folder}")
    if args.interval <= 0:
        parser.error("--interval must be greater than zero")

    try:
        watch(folder, args.interval)
    except KeyboardInterrupt:
        print("\nStopped.")


if __name__ == "__main__":
    main()
