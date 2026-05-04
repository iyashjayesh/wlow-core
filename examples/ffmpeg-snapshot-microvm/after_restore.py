import random
import sys
import time


def main() -> None:
    random.seed(time.time_ns())
    sys.stdout.write("after_restore_ok\n")
    sys.stdout.flush()


if __name__ == "__main__":
    main()
