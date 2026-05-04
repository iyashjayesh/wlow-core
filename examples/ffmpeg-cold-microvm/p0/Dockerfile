FROM python:3.12-alpine

RUN apk add --no-cache ffmpeg

WORKDIR /app
COPY processor.py /app/processor.py
RUN ffmpeg -y -f lavfi -i color=c=red:s=128x128 -frames:v 1 -update 1 /app/left.png \
 && ffmpeg -y -f lavfi -i color=c=blue:s=128x128 -frames:v 1 -update 1 /app/right.png

ENTRYPOINT ["python", "/app/processor.py"]
