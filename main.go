package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cast"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	commonerrors "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	lighthouse "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/lighthouse/v20200324"
)

func main() {
	pterm.DefaultHeader.WithFullWidth().Println("腾讯云轻量秒杀工具")
	pterm.DefaultCenter.Println("⚠️ 开始前请确保腾讯云账号有足够的余额 ⚠️")
	id, _ := pterm.DefaultInteractiveTextInput.WithMultiLine(false).Show("SecretId")
	key, _ := pterm.DefaultInteractiveTextInput.WithMultiLine(false).WithMask("*").Show("SecretKey")

	spinner, _ := pterm.DefaultSpinner.Start("正在验证账号信息...")

	credential := common.NewCredential(id, key)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "lighthouse.tencentcloudapi.com"
	client, _ := lighthouse.NewClient(credential, "ap-hongkong", cpf)
	response, err := client.DescribeBundles(lighthouse.NewDescribeBundlesRequest())
	var sdkError *commonerrors.TencentCloudSDKError
	if err != nil {
		if errors.As(err, &sdkError) {
			spinner.Fail(fmt.Sprintf("验证失败：%v", sdkError.GetMessage()))
			return
		}
		spinner.Fail(fmt.Sprintf("验证失败：%v", err))
		return
	}

	spinner.Success("验证成功！")

	var options []string
	var bundleIds []string
	for _, bundle := range response.Response.BundleSet {
		options = append(options, strings.Join([]string{
			*bundle.BundleTypeDescription,
			cast.ToString(*bundle.CPU) + "C",
			cast.ToString(*bundle.Memory) + "G",
			cast.ToString(*bundle.InternetMaxBandwidthOut) + "Mbps",
			cast.ToString(*bundle.SystemDiskSize) + "GB",
			"￥" + cast.ToString(*bundle.Price.InstancePrice.DiscountPrice)},
			"-"))
		bundleIds = append(bundleIds, *bundle.BundleId)
	}

	selected, _ := pterm.DefaultInteractiveSelect.WithMaxHeight(10).WithOptions(options).Show("请选择需要秒杀的套餐")
	bundleId := bundleIds[slices.Index(options, selected)]

	spinner, _ = pterm.DefaultSpinner.Start("正在秒杀中...")

	rateLimiter := NewRateLimiter(4, 1)
	successChan := make(chan bool)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动秒杀循环
	go func() {
		count := 0

		for {
			select {
			case <-ctx.Done():
				return
			default:
				if err = rateLimiter.Wait(ctx); err != nil {
					continue
				}

				delay := time.Duration(rand.IntN(200)) * time.Millisecond
				time.Sleep(delay)

				count++
				pterm.FgYellow.Printfln("正在进行第 %d 次秒杀... (%dms)",
					count, delay.Milliseconds())

				if err = createInstance(client, bundleId); err != nil {
					if count%10 == 0 { // 每10次才打印一次完整错误信息
						pterm.FgRed.Printfln("第 %d 次秒杀失败：%v", count, err)
					}
				} else {
					successChan <- true
					return
				}
			}
		}
	}()

	// 等待秒杀结果或用户中断
	select {
	case <-successChan:
		spinner.Success("秒杀成功！")
	case <-sigChan:
		spinner.Fail("秒杀被用户中断")
		cancel()
	}
}

func createInstance(client *lighthouse.Client, bundleId string) error {
	request := lighthouse.NewCreateInstancesRequest()
	request.BundleId = common.StringPtr(bundleId)
	request.BlueprintId = common.StringPtr("lhbp-1l4ptuvm") // ubuntu 24.04
	request.InstanceChargePrepaid = &lighthouse.InstanceChargePrepaid{
		Period: common.Int64Ptr(1),
	}
	request.LoginConfiguration = &lighthouse.LoginConfiguration{
		AutoGeneratePassword: common.StringPtr("YES"),
	}
	request.AutoVoucher = common.BoolPtr(true)

	_, err := client.CreateInstances(request)
	var sdkError *commonerrors.TencentCloudSDKError
	if err != nil {
		if errors.As(err, &sdkError) {
			return errors.New(sdkError.GetMessage())
		}
		return err
	}

	return nil
}
