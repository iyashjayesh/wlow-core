import gc
import sys


def main() -> None:
    gc.collect()
    sys.stdout.write("before_snapshot_ok\n")
    sys.stdout.flush()


if __name__ == "__main__":
    main()
