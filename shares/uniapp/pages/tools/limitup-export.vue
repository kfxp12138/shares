<template>
  <view class="limitup-page">
    <scroll-view scroll-y class="scroll">
      <view class="hero">
        <view class="hero-tag">同概念涨停 Excel 下载</view>
        <view class="hero-title">一键匹配股票池概念 · 获取精美报表</view>
        <view class="hero-sub">上传自选股池，系统将筛出当日涨停股并按概念维度排序，生成包含连板、近5/10日涨幅等指标的 Excel。</view>
      </view>

      <view class="card upload-card">
        <view class="card-head">
          <view class="title">上传股票池</view>
          <view class="subtitle">支持 .xlsx / .csv / .txt 文件，最多 5MB</view>
        </view>
        <view class="card-body">
          <view class="upload-area" @tap="handleUploadTap">
            <view class="upload-icon">
              <view class="icon-circle">UP</view>
            </view>
            <view class="upload-text">
              <text class="main">拖拽文件到此处，或点击选择</text>
              <text class="hint">至少包含第一列股票代码，允许同一单元格多个代码</text>
            </view>
            <view class="upload-status" v-if="fileName">
              <text class="label">已选择：</text>
              <text class="file">{{ fileName }}</text>
            </view>
          </view>
          <!-- H5: 使用原生 input 触发 -->
          <!-- #ifdef H5 -->
          <input ref="h5File" class="hidden-input" type="file" accept=".xlsx,.xlsm,.xltx,.xltm,.csv,.txt" @change="onFileChange" />
          <!-- #endif -->
          <!-- 非 H5 平台提示 -->
          <!-- #ifndef H5 -->
          <view class="tips-mini">移动端暂不支持直接下载 Excel，请在浏览器版本使用。</view>
          <!-- #endif -->

          <view class="meta-row">
            <view class="meta-block">
              <text class="label">统计口径</text>
              <text class="value">按最新交易日涨停股票自动匹配概念</text>
            </view>
            <view class="meta-block">
              <text class="label">自定义日期</text>
              <picker mode="date" :value="selectedDate" @change="onDateChange">
                <view class="date-picker" :class="{ placeholder: !selectedDate }">
                  <text v-if="selectedDate">{{ selectedDate }}</text>
                  <text v-else>默认最新交易日</text>
                </view>
              </picker>
            </view>
          </view>
        </view>
        <view class="card-footer">
          <button class="action-btn" :class="{ disabled: submitting }" :disabled="submitting" @tap="submit">
            <text v-if="!submitting">生成 Excel 报表</text>
            <text v-else>生成中...</text>
          </button>
          <view class="safety-note">数据仅用于即时匹配，不会被存储。</view>
        </view>
      </view>

      <view class="card how-card">
        <view class="card-head">
          <view class="title">报表亮点</view>
        </view>
        <view class="features">
          <view class="feature">
            <view class="icon gradient-1">①</view>
            <view class="text-block">
              <view class="f-title">概念维度排序</view>
              <view class="f-desc">优先展示连板数、近五日/前三日涨停次数、概念涨幅等关键指标。</view>
            </view>
          </view>
          <view class="feature">
            <view class="icon gradient-2">②</view>
            <view class="text-block">
              <view class="f-title">多维度指标</view>
              <view class="f-desc">提供个股层面的连板统计、5/10日涨幅，概念层面均值/最大值对比。</view>
            </view>
          </view>
          <view class="feature">
            <view class="icon gradient-3">③</view>
            <view class="text-block">
              <view class="f-title">即下即用</view>
              <view class="f-desc">下载即得 xlsx 文件，可直接用于复盘或分享。</view>
            </view>
          </view>
        </view>
      </view>
    </scroll-view>
  </view>
</template>

<script>
export default {
  data() {
    return {
      selectedFile: null,
      selectedDate: '',
      submitting: false
    }
  },
  computed: {
    fileName() {
      return this.selectedFile ? this.selectedFile.name : ''
    }
  },
  methods: {
    handleUploadTap() {
      // H5 直接触发 input
      // #ifdef H5
      this.$refs.h5File && this.$refs.h5File.click()
      // #endif
      // #ifndef H5
      uni.showToast({
        title: '请在 PC 浏览器打开使用',
        icon: 'none'
      })
      // #endif
    },
    onFileChange(e) {
      const [file] = e.target.files
      if (!file) return
      if (file.size > 5 * 1024 * 1024) {
        uni.showToast({ title: '文件需小于 5MB', icon: 'none' })
        return
      }
      this.selectedFile = file
    },
    onDateChange(e) {
      this.selectedDate = e.detail.value
    },
    async submit() {
      if (this.submitting) return
      if (!this.selectedFile) {
        uni.showToast({ title: '请先选择股票池文件', icon: 'none' })
        return
      }
      this.submitting = true
      try {
        if (typeof window === 'undefined' || !window.fetch) {
          uni.showToast({ title: '当前平台暂不支持下载', icon: 'none' })
          return
        }
        const formData = new FormData()
        formData.append('file', this.selectedFile)
        if (this.selectedDate) formData.append('date', this.selectedDate)

        const resp = await fetch('/shares/api/v1/analy.limitup_pool_export', {
          method: 'POST',
          body: formData
        })
        if (!resp.ok) {
          const text = await resp.text()
          throw new Error(text || '导出失败')
        }
        const blob = await resp.blob()
        const fileName = this.buildFileName()
        const link = document.createElement('a')
        link.href = URL.createObjectURL(blob)
        link.download = fileName
        document.body.appendChild(link)
        link.click()
        document.body.removeChild(link)
        URL.revokeObjectURL(link.href)
        uni.showToast({ title: '已开始下载', icon: 'success' })
      } catch (err) {
        console.error(err)
        uni.showToast({ title: err.message || '生成失败', icon: 'none', duration: 2500 })
      } finally {
        this.submitting = false
      }
    },
    buildFileName() {
      const datePart = this.selectedDate ? this.selectedDate.replace(/-/g, '') : 'latest'
      return `limitup_concepts_${datePart}.xlsx`
    }
  }
}
</script>

<style scoped lang="scss">
.limitup-page {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background: linear-gradient(180deg, #f9fbff 0%, #ffffff 35%);
  padding: 48rpx 32rpx 56rpx;
  box-sizing: border-box;
}

.scroll {
  flex: 1;
}

.hero {
  margin-bottom: 32rpx;
  padding: 40rpx 48rpx;
  border-radius: 32rpx;
  background: linear-gradient(135deg, #3d7eff 0%, #6c5cff 100%);
  color: #fff;
  box-shadow: 0 26rpx 60rpx rgba(61, 126, 255, 0.26);
  .hero-tag {
    display: inline-flex;
    padding: 6rpx 26rpx;
    border-radius: 100rpx;
    background: rgba(255, 255, 255, 0.18);
    font-size: 24rpx;
    letter-spacing: 6rpx;
    margin-bottom: 20rpx;
  }
  .hero-title {
    font-size: 48rpx;
    font-weight: 600;
    line-height: 1.4;
    margin-bottom: 12rpx;
  }
  .hero-sub {
    font-size: 28rpx;
    line-height: 1.6;
    opacity: 0.85;
  }
}

.card {
  background: #fff;
  border-radius: 28rpx;
  margin-bottom: 32rpx;
  box-shadow: 0 14rpx 48rpx rgba(24, 39, 75, 0.08);
  overflow: hidden;
}

.card-head {
  padding: 32rpx 36rpx 0;
  .title {
    font-size: 36rpx;
    font-weight: 600;
    color: #20264b;
    margin-bottom: 8rpx;
  }
  .subtitle {
    font-size: 26rpx;
    color: #7a8099;
  }
}

.card-body {
  padding: 32rpx 36rpx 12rpx;
}

.upload-area {
  border: 2rpx dashed #8aa4ff;
  border-radius: 24rpx;
  padding: 40rpx 30rpx;
  display: flex;
  align-items: center;
  position: relative;
  transition: all 0.2s ease;
  background: rgba(138, 164, 255, 0.08);
  &:active {
    transform: scale(0.99);
  }
    .upload-icon {
      margin-right: 28rpx;
      .icon-circle {
        width: 92rpx;
        height: 92rpx;
        border-radius: 50%;
        background: linear-gradient(135deg, #6d8bff 0%, #51a8ff 100%);
        display: flex;
        align-items: center;
        justify-content: center;
        color: #fff;
        font-size: 28rpx;
        font-weight: 600;
        letter-spacing: 4rpx;
      }
    }
  .upload-text {
    flex: 1;
    .main {
      display: block;
      font-size: 30rpx;
      color: #2c3170;
      font-weight: 600;
      margin-bottom: 10rpx;
    }
    .hint {
      font-size: 24rpx;
      color: #8a8fb0;
    }
  }
  .upload-status {
    position: absolute;
    bottom: 20rpx;
    right: 24rpx;
    background: rgba(61, 126, 255, 0.1);
    border-radius: 100rpx;
    padding: 10rpx 20rpx;
    font-size: 22rpx;
    color: #3d7eff;
    display: flex;
    align-items: center;
    .file {
      max-width: 260rpx;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .label {
      margin-right: 12rpx;
    }
  }
}

.hidden-input {
  display: none;
}

.tips-mini {
  margin-top: 20rpx;
  padding: 22rpx;
  background: rgba(255, 192, 0, 0.12);
  border-radius: 18rpx;
  font-size: 24rpx;
  color: #b58100;
}

.meta-row {
  display: flex;
  margin-top: 32rpx;
  flex-wrap: wrap;
  .meta-block {
    flex: 1;
    min-width: 280rpx;
    margin-right: 24rpx;
    margin-bottom: 20rpx;
    .label {
      font-size: 24rpx;
      color: #7681a1;
      margin-bottom: 12rpx;
      display: block;
    }
    .value {
      font-size: 26rpx;
      color: #2c3170;
    }
  }
  .meta-block:last-child {
    margin-right: 0;
  }
}

.date-picker {
  padding: 22rpx 28rpx;
  background: #f4f6ff;
  border-radius: 18rpx;
  color: #2c3170;
  font-size: 26rpx;
  &.placeholder {
    color: #9aa2c4;
  }
}

.card-footer {
  padding: 0 36rpx 32rpx;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  .action-btn {
    width: 100%;
    background: linear-gradient(135deg, #3d7eff 0%, #6c5cff 100%);
    color: #fff;
    font-size: 30rpx;
    padding: 28rpx 0;
    border-radius: 18rpx;
    text-align: center;
    font-weight: 600;
    box-shadow: 0 18rpx 40rpx rgba(61, 126, 255, 0.26);
    &.disabled {
      background: #ccd5ff;
      box-shadow: none;
    }
  }
  .safety-note {
    margin-top: 20rpx;
    font-size: 24rpx;
    color: #98a0bf;
  }
}

.features {
  padding: 12rpx 36rpx 36rpx;
  .feature {
    display: flex;
    align-items: flex-start;
    padding: 22rpx 0;
    border-bottom: 1rpx solid #f0f2f9;
    &:last-child {
      border-bottom: none;
    }
    .icon {
      width: 72rpx;
      height: 72rpx;
      border-radius: 24rpx;
      display: flex;
      align-items: center;
      justify-content: center;
      margin-right: 22rpx;
      color: #fff;
      font-weight: 600;
      font-size: 30rpx;
    }
    .gradient-1 { background: linear-gradient(135deg, #6d8bff 0%, #51a8ff 100%); }
    .gradient-2 { background: linear-gradient(135deg, #6c5cff 0%, #a461ff 100%); }
    .gradient-3 { background: linear-gradient(135deg, #ff7d7d 0%, #ffb661 100%); }
    .text-block {
      flex: 1;
      .f-title {
        font-size: 30rpx;
        color: #2c3170;
        font-weight: 600;
        margin-bottom: 6rpx;
      }
      .f-desc {
        font-size: 26rpx;
        color: #7a8099;
        line-height: 1.5;
      }
    }
  }
}

@media (min-width: 1024px) {
  .limitup-page {
    max-width: 1040px;
    margin: 0 auto;
  }
}
</style>
