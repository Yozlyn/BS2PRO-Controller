'use client';

import React, { useState, useEffect } from 'react';
import { InformationCircleIcon } from '@heroicons/react/24/outline';
import { apiService } from '../services/api';
import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime';
import { Card } from './ui';

const FanHex = ({ className }: { className?: string }) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" className={className}>
    <polygon points="12 2 20.66 7 20.66 17 12 22 3.34 17 3.34 7" strokeOpacity="0.4" />
    <circle cx="12" cy="12" r="3" />
    <path d="M12 8L12 2M16 12L22 12M12 16L12 22M8 12L2 12" />
    <path d="M14.8 9.2L18.5 5.5M14.8 14.8L18.5 18.5M9.2 14.8L5.5 18.5M9.2 9.2L5.5 5.5" strokeOpacity="0.4"/>
  </svg>
);

export default function AboutPanel() {
  const [appVersion, setAppVersion] = useState('');
  const [iframeLoaded, setIframeLoaded] = useState(false);

  useEffect(() => {
    apiService.getAppVersion()
      .then((version) => setAppVersion(version || ''))
      .catch(() => setAppVersion(''));
  }, []);

  const handleOpenUrl = (url: string) => {
    BrowserOpenURL(url);
  };

  return (
    <div className="w-full flex flex-col animate-in fade-in duration-300">
      {/* 顶部标题和版本信息 */}
      <div className="flex flex-col items-center justify-center py-8 group">
        <FanHex className="w-20 h-20 text-blue-500/80 mb-6 transition-all duration-500 animate-[spin_12s_linear_infinite] group-hover:animate-[spin_2s_linear_infinite] group-hover:text-blue-400" />
        <h2 className="font-black text-2xl text-slate-900 dark:text-gray-300 tracking-widest">
          BS2PRO <span className="text-blue-500">CONTROLLER</span>
        </h2>
        <p className="font-medium text-sm text-slate-600 dark:text-slate-400 mt-2 bg-slate-100 dark:bg-slate-800 px-4 py-1.5 rounded-full border border-slate-200 dark:border-slate-700">
          版本 {appVersion || '1.0.0'}
        </p>
        <div className="w-12 h-1 bg-blue-500 rounded-full my-8"></div>
        
        {/* 开发者信息 - 以简单样式显示 */}
        <div className="text-center">
          <div className="flex items-center justify-center gap-3">
            <img
              src="https://q1.qlogo.cn/g?b=qq&nk=507249007&s=640"
              alt="开发者头像"
              className="w-10 h-10 rounded-full border-2 border-white shadow"
            />
            <div className="text-left">
              <div className="font-bold text-base text-slate-800 dark:text-gray-300 tracking-widest">TIANLI</div>
              <button
                onClick={() => handleOpenUrl('mailto:wutianli@tianli0.top')}
                className="text-sm text-blue-600 dark:text-blue-400 hover:underline"
              >
                wutianli@tianli0.top
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* 关于和更新卡片 */}
      <Card className="mt-6">
        <div className="rounded-2xl border border-gray-200 dark:border-gray-700 overflow-hidden bg-white dark:bg-gray-800">
          <div className="px-4 py-3 bg-gray-50 dark:bg-gray-700/50 border-b border-gray-200 dark:border-gray-600">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <InformationCircleIcon className="w-5 h-5 text-blue-600 dark:text-blue-400" />
                <span className="font-medium text-gray-900 dark:text-gray-300">关于 & 更新</span>
              </div>
              <button
                onClick={() => handleOpenUrl('https://blog.tianli0.top/pages/bs2pro')}
                className="text-xs text-blue-600 dark:text-blue-400 hover:underline"
              >
                在浏览器中打开
              </button>
            </div>
          </div>
          <div className="relative h-80">
            <iframe
              src="https://blog.tianli0.top/pages/bs2pro"
              className="w-full h-full border-0"
              title="BS2PRO 关于页面"
              sandbox="allow-scripts allow-same-origin allow-popups allow-forms"
              loading="lazy"
              onLoad={() => setIframeLoaded(true)}
            />
            {!iframeLoaded && (
              <div className="absolute inset-0 flex items-center justify-center bg-gray-50 dark:bg-gray-800">
                <div className="animate-spin w-8 h-8 border-4 border-blue-600 border-t-transparent rounded-full" />
              </div>
            )}
          </div>
        </div>


      </Card>

      {/* 版权信息 */}
      <div className="mt-8 text-center">
        <p className="text-xs text-gray-500 dark:text-gray-400">
          © 2024 TIANLI. BS2PRO Controller 版权所有。
        </p>
        <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
          本软件为开源项目，遵循 MIT 许可证。
        </p>
      </div>
    </div>
  );
}