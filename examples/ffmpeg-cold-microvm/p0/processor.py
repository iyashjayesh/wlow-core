# Analyzer: mypy --strict and pylint.
import json
import subprocess
import sys
from typing import Any


def main() -> None:
    req: dict[str, Any] = json.load(sys.stdin)
    left = str(req["left"])
    right = str(req["right"])
    output = str(req.get("output", "/tmp/flamflow-hstack.png"))
    subprocess.run(
        [
            "ffmpeg",
            "-y",
            "-i",
            left,
            "-i",
            right,
            "-filter_complex",
            "hstack=inputs=2",
            "-frames:v",
            "1",
            "-update",
            "1",
            output,
        ],
        check=True,
    )
    print(json.dumps({"processor": "p0", "layout": "hstack", "output": output}))


if __name__ == "__main__":
    main()
