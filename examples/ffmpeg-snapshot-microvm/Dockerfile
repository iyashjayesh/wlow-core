FROM python:3.12-slim

RUN apt-get update \
 && apt-get install -y --no-install-recommends ffmpeg \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY server.py /app/server.py
COPY before_snapshot.py /app/before_snapshot.py
COPY after_restore.py /app/after_restore.py

ENTRYPOINT ["python", "/app/server.py"]
