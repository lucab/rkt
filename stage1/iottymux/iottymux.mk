ASSCB_STATIC := false
ASSCB_EXTRA_CFLAGS := -D_GNU_SOURCE `pkg-config --cflags libsystemd`
ASSCB_EXTRA_LDFLAGS := `pkg-config --libs libsystemd`

include stage1/makelib/aci_simple_static_c_bin.mk
