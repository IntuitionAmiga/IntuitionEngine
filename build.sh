# build.sh

echo Intuition Engine Virtual Machine and IE32Asm assembler build script
mkdir -p ./bin
go mod tidy -v
echo Building ./bin/IntuitionEngine...
CGO_JOBS=$(nproc) nice -19 go build -v -ldflags "-s -w" .
echo Super stripping debug symbols from ./bin/IntuitionEngine...
nice -19 sstrip -z IntuitionEngine >/dev/null
echo UPX compressing ./IntuitionEngine
nice -19 upx --lzma IntuitionEngine >/dev/null
mv IntuitionEngine ./bin
echo Building ./bin/ie32asm...
go build -v -ldflags "-s -w" assembler/ie32asm.go
echo Super stripping debug symbols from ./bin/ie32asm...
sstrip -z ie32asm >/dev/null
upx --lzma ie32asm >/dev/null
mv ie32asm ./bin
echo ls -alh ./bin/
ls -alh ./bin
