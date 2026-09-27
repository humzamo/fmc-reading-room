#!/usr/bin/env python3
"""One-off tool: commit and push proceedings/ in small batches instead of
one ~6.5GB commit+push. Safe to re-run after any failure (network, auth,
timeout) — it re-derives what's still untracked each time and continues
from there, rather than assuming its own prior progress.
"""
import argparse
import os
import subprocess
import sys

DEFAULT_BATCH_SIZE = 100
BRANCH = "main"


def run(cmd):
    subprocess.run(cmd, check=True)


def untracked_proceedings_files():
    """Every file under proceedings/ git doesn't know about yet, in the
    stable order `git ls-files` returns. NUL-delimited so filenames with
    spaces/commas/parens (all over this dataset) split safely.
    """
    out = subprocess.run(
        ["git", "ls-files", "--others", "--exclude-standard", "-z", "--", "proceedings/"],
        check=True, capture_output=True,
    ).stdout
    return [os.fsdecode(f) for f in out.split(b"\0") if f]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--batch-size", type=int, default=DEFAULT_BATCH_SIZE)
    parser.add_argument("--dry-run", action="store_true", help="show the batch plan without committing or pushing")
    args = parser.parse_args()

    repo_root = subprocess.run(
        ["git", "rev-parse", "--show-toplevel"], check=True, capture_output=True, text=True,
    ).stdout.strip()
    os.chdir(repo_root)

    # Computed once per script invocation: re-running the script (e.g. after
    # a failed push) starts a fresh process, which recomputes this list and
    # naturally excludes whatever earlier batches already got committed —
    # that's the whole resumability story, no in-run re-querying needed.
    files = untracked_proceedings_files()
    total_files = len(files)
    if total_files == 0:
        print("Nothing to do — proceedings/ is already fully committed.")
        return

    total_batches = (total_files + args.batch_size - 1) // args.batch_size
    print(f"{total_files} untracked files under proceedings/, in batches of {args.batch_size} ({total_batches} batches).")

    for batch_num, start in enumerate(range(0, total_files, args.batch_size), start=1):
        batch = files[start : start + args.batch_size]
        committed_so_far = start + len(batch)

        print(f"\nBatch {batch_num}/{total_batches}: {len(batch)} files ({committed_so_far}/{total_files})")

        if args.dry_run:
            for f in batch[:3]:
                print("  would add:", f)
            if len(batch) > 3:
                print(f"  ... and {len(batch) - 3} more")
            continue

        run(["git", "add", "--", *batch])
        run(["git", "commit", "-q", "-m", f"Add proceedings documents (batch {batch_num}/{total_batches})"])

        result = subprocess.run(["git", "push", "origin", BRANCH])
        if result.returncode != 0:
            print(f"\nPush failed on batch {batch_num}. The commit is saved locally.", file=sys.stderr)
            print("Fix whatever's wrong (network, auth, etc.) and re-run this script — it resumes automatically.", file=sys.stderr)
            sys.exit(1)

    if not args.dry_run:
        print(f"\nDone: {total_batches} batches, {total_files} files committed and pushed.")


if __name__ == "__main__":
    main()
