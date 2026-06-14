package main

import (
	"debug/elf"
	"encoding/binary"
	"os"
	"regexp"
	"strings"
	"testing"
)

const ieBSDSocketDispatchPath = "../AROS-deadw00d/arch/m68k-ie/libs/bsdsocket/bsdsocket_dispatch.c"
const ieBSDSocketConfigPath = "../AROS-deadw00d/arch/m68k-ie/libs/bsdsocket/bsdsocket.conf"
const ieAROSRomELFPath = "../AROS-deadw00d/bin/ie-m68k/bin/ie-m68k/gen/boot/aros-ie-m68k-rom.elf"

func TestIEBSDSocketSocketBaseTagListCompatibility(t *testing.T) {
	srcBytes, err := os.ReadFile(ieBSDSocketDispatchPath)
	if err != nil {
		t.Skipf("IE AROS bsdsocket source not available: %v", err)
	}
	src := string(srcBytes)

	for _, want := range []string{
		"static LONG ieb_fd_callback(struct IEBSDSocketBase *base, LONG fd, LONG action)",
		"static LONG ieb_socket_close_host(struct IEBSDSocketBase *base, LONG s)",
		"static LONG ieb_socket_release_host(struct IEBSDSocketBase *base, LONG s, LONG id)",
		"static LONG ieb_socket_release_copy_host(struct IEBSDSocketBase *base, LONG s, LONG id)",
		"static LONG ieb_socket_validate_fd(struct IEBSDSocketBase *base, LONG fd)",
		"static struct TagItem *ieb_next_tag_item(struct TagItem **tagList)",
		"case TAG_IGNORE:",
		"case TAG_SKIP:",
		"case TAG_MORE:",
		"return SocketBase->dTableSize;",
		"if (nfds < 0 || nfds > (int)SocketBase->dTableSize)",
		"if ((ULONG)value > 0 && (ULONG)value <= IEBSD_DTABLE_SIZE)",
		"static LONG ieb_socket_finish_release(struct IEBSDSocketBase *base, LONG fd, LONG id)",
		"return ieb_socket_finish_alloc(SocketBase, fd)",
		"return ieb_socket_finish_alloc(SocketBase, ieb_socket_call(SocketBase, IE_SOCK_CMD_ACCEPT, &req))",
		"return ieb_socket_finish_free(SocketBase, s)",
		"if (fd1 == fd2)",
		"return fd2;",
		"if (fd1 != -1 && !SocketBase->dTableUsed[fd1])",
		"if (fd2 != -1)",
		"if (SocketBase->dTableUsed[fd2])",
		"if ((error = ieb_socket_finish_free(SocketBase, fd2)) != 0)",
		"if ((error = ieb_fd_callback(SocketBase, fd2, FDCB_CHECK)) != 0)",
		"return ieb_socket_finish_alloc(SocketBase, newfd)",
		"base->dTableUsed[fd] = TRUE;",
		"base->dTableUsed[fd] = FALSE;",
		"return ieb_socket_finish_release(SocketBase, sd, id)",
		"return ieb_socket_release_copy(SocketBase, sd, id)",
		"IE_SOCK_CMD_RELEASE",
		"IE_SOCK_CMD_RELEASECOPY",
		"IE_SOCK_CMD_OBTAIN",
		"tagCode = (UWORD)(tag->ti_Tag & ~SBTF_REF)",
		"tagData = (tag->ti_Tag & SBTF_REF) ? (IPTR *)tag->ti_Data : &tag->ti_Data",
		"case (SBTC_BREAKMASK << SBTB_CODE) | SBTF_SET:",
		"case (SBTC_FDCALLBACK << SBTB_CODE) | SBTF_SET:",
		"case (SBTC_LOGSTAT << SBTB_CODE) | SBTF_SET:",
		"case (SBTC_LOGTAGPTR << SBTB_CODE) | SBTF_SET:",
		"case (SBTC_LOGFACILITY << SBTB_CODE) | SBTF_SET:",
		"case (SBTC_LOGMASK << SBTB_CODE) | SBTF_SET:",
		"case SBTC_ERRNOSTRPTR << SBTB_CODE:",
		"case SBTC_HERRNOSTRPTR << SBTB_CODE:",
		"case SBTC_HAVE_DNS_API << SBTB_CODE:",
		"case SBTC_HAVE_ADDRESS_CONVERSION_API << SBTB_CODE:",
		"case SBTC_HAVE_ROUTING_API << SBTB_CODE:",
		"case SBTC_HAVE_INTERFACE_API << SBTB_CODE:",
		"case SBTC_HAVE_MONITORING_API << SBTB_CODE:",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("bsdsocket SocketBaseTagList missing compatibility fragment %q", want)
		}
	}

	if strings.Contains(src, "NextTagItem(") {
		t.Fatalf("bsdsocket ROM library must not depend on utility.library NextTagItem")
	}
	obtainSocket := regexp.MustCompile(`(?s)AROS_LH4\(LONG, ObtainSocket,.*?AROS_LIBFUNC_EXIT\n\}`).FindString(src)
	if obtainSocket == "" {
		t.Fatalf("ObtainSocket implementation missing")
	}
	if strings.Contains(obtainSocket, "ieb_socket_validate_fd(SocketBase, id)") || strings.Contains(obtainSocket, "SocketBase->dTableUsed[id]") {
		t.Fatalf("ObtainSocket must treat id as a release key, not a descriptor")
	}
	if !strings.Contains(obtainSocket, "req.a[IEBSD_REQ_AUX1] = id;") ||
		!strings.Contains(obtainSocket, "req.a[IEBSD_REQ_DOMAIN] = domain;") ||
		!strings.Contains(obtainSocket, "req.a[IEBSD_REQ_TYPE] = type;") ||
		!strings.Contains(obtainSocket, "req.a[IEBSD_REQ_PROTOCOL] = protocol;") ||
		!strings.Contains(obtainSocket, "return ieb_socket_finish_alloc(SocketBase, ieb_socket_call(SocketBase, IE_SOCK_CMD_OBTAIN, &req));") {
		t.Fatalf("ObtainSocket must ask the host bridge to obtain a released socket and allocate the returned descriptor")
	}

	releaseCopy := regexp.MustCompile(`(?s)static LONG ieb_socket_release_copy\(.*?\n\}`).FindString(src)
	if releaseCopy == "" {
		t.Fatalf("ReleaseCopyOfSocket helper missing")
	}
	if strings.Contains(releaseCopy, "FDCB_FREE") || strings.Contains(releaseCopy, "ieb_socket_close_host") || strings.Contains(releaseCopy, "ieb_socket_finish_free") {
		t.Fatalf("ReleaseCopyOfSocket must not close/deallocate the caller's descriptor")
	}
	if strings.Contains(releaseCopy, "return (error < 0) ? error : id;") {
		t.Fatalf("ReleaseCopyOfSocket must return the host-provided release key, including generated keys")
	}
	if !strings.Contains(releaseCopy, "return ieb_socket_release_copy_host(base, fd, id);") {
		t.Fatalf("ReleaseCopyOfSocket must preserve a host-side copy and return the host-provided release key")
	}

	releaseSocket := regexp.MustCompile(`(?s)AROS_LH2\(LONG, ReleaseSocket,.*?AROS_LIBFUNC_EXIT\n\}`).FindString(src)
	if !strings.Contains(releaseSocket, "return ieb_socket_finish_release(SocketBase, sd, id);") {
		t.Fatalf("ReleaseSocket must deallocate the caller's descriptor and return the release id")
	}
	releaseCopyPublic := regexp.MustCompile(`(?s)AROS_LH2\(LONG, ReleaseCopyOfSocket,.*?AROS_LIBFUNC_EXIT\n\}`).FindString(src)
	if !strings.Contains(releaseCopyPublic, "return ieb_socket_release_copy(SocketBase, sd, id);") {
		t.Fatalf("ReleaseCopyOfSocket must preserve the caller's descriptor")
	}
	releaseHelper := regexp.MustCompile(`(?s)static LONG ieb_socket_finish_release\(.*?\n\}`).FindString(src)
	if !strings.Contains(releaseHelper, "releaseId = ieb_socket_release_host(base, fd, id);") ||
		!strings.Contains(releaseHelper, "return releaseId;") {
		t.Fatalf("ReleaseSocket must return the host-provided release key, including generated keys")
	}
	hostReleaseIdx := strings.Index(releaseHelper, "releaseId = ieb_socket_release_host(base, fd, id);")
	callbackFreeIdx := strings.Index(releaseHelper, "ieb_fd_callback(base, fd, FDCB_FREE)")
	if hostReleaseIdx < 0 || callbackFreeIdx < 0 || callbackFreeIdx < hostReleaseIdx {
		t.Fatalf("ReleaseSocket must not call FDCB_FREE before host release succeeds")
	}
	afterCommit := releaseHelper[hostReleaseIdx:]
	if strings.Contains(afterCommit, "return ieb_socket_fail(base, error)") {
		t.Fatalf("ReleaseSocket must not report callback failure after host release has committed")
	}
	dup2Socket := regexp.MustCompile(`(?s)AROS_LH2\(int, Dup2Socket,.*?AROS_LIBFUNC_EXIT\n\}`).FindString(src)
	if !strings.Contains(dup2Socket, "return ieb_socket_finish_alloc(SocketBase, newfd);") {
		t.Fatalf("Dup2Socket must mark successful duplicate descriptors allocated")
	}
	if !strings.Contains(dup2Socket, "if (fd1 == fd2)") || !strings.Contains(dup2Socket, "return fd2;") {
		t.Fatalf("Dup2Socket(fd, fd) must be a no-op success")
	}
	if !strings.Contains(dup2Socket, "if (fd1 != -1 && !SocketBase->dTableUsed[fd1])") {
		t.Fatalf("Dup2Socket must validate fd1 is active before touching fd2")
	}
}

func TestIEBSDSocketModuleConfigHandlesLibrarySymbolSets(t *testing.T) {
	srcBytes, err := os.ReadFile(ieBSDSocketConfigPath)
	if err != nil {
		t.Skipf("IE AROS bsdsocket config not available: %v", err)
	}
	src := string(srcBytes)

	if strings.Contains(src, "noautolib") {
		t.Fatalf("bsdsocket.conf must not use noautolib; generated ROM module needs LIBS symbol-set handling")
	}
	if !strings.Contains(src, "options noautoinit") {
		t.Fatalf("bsdsocket.conf must keep noautoinit for explicit IE library init")
	}
}

func TestIEBSDSocketRomInitTableAllocatesPrivateBase(t *testing.T) {
	f, err := elf.Open(ieAROSRomELFPath)
	if err != nil {
		t.Skipf("IE AROS ROM ELF not available: %v", err)
	}
	defer f.Close()

	symbols, err := f.Symbols()
	if err != nil {
		t.Skipf("IE AROS ROM ELF symbols not available: %v", err)
	}

	var initTable *elf.Symbol
	for i := range symbols {
		if symbols[i].Name == "BSDSocket_InitTable" {
			initTable = &symbols[i]
			break
		}
	}
	if initTable == nil {
		t.Fatalf("BSDSocket_InitTable symbol missing from ROM ELF")
	}

	for _, section := range f.Sections {
		if initTable.Value < section.Addr || initTable.Value+4 > section.Addr+section.Size {
			continue
		}
		data, err := section.Data()
		if err != nil {
			t.Fatalf("read ROM section %s: %v", section.Name, err)
		}
		offset := initTable.Value - section.Addr
		baseSize := binary.BigEndian.Uint32(data[offset : offset+4])
		if baseSize < 490 {
			t.Fatalf("bsdsocket ROM init table base size=%d, want at least 490 for current IEBSDSocketBase", baseSize)
		}
		return
	}
	t.Fatalf("BSDSocket_InitTable address %#x not contained in any ROM ELF section", initTable.Value)
}
