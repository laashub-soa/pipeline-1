subinclude("///pleasings2//go:compat")

go_library(
    name = "autoscaling",
    srcs = glob(
        ["*.go"],
        exclude = ["*_test.go"],
    ),
    visibility = ["PUBLIC"],
    deps = [
        "//pkg/providers/amazon",
        "//pkg/providers/amazon/ec2",
    ],
)
