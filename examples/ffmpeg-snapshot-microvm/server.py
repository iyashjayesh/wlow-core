import json
import subprocess
import sys


def handle(req: dict) -> dict:
    left = req["left"]
    right = req["right"]
    output = req.get("output", "/tmp/flamflow-ffmpeg-output.jpg")
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
            output,
        ],
        check=True,
    )
    return {"output": output}


def main() -> None:
    print("FLAMFLOW_READY", flush=True)
    for line in sys.stdin:
        if not line.strip():
            continue
        try:
            response = handle(json.loads(line))
            print(json.dumps(response), flush=True)
        except Exception as exc:
            print(json.dumps({"error": str(exc)}), flush=True)


if __name__ == "__main__":
    main()
