package com.proxy.socks5frp

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.net.wifi.WifiManager
import android.os.Build
import android.os.IBinder
import android.os.PowerManager
import androidx.core.app.NotificationCompat
import proxycore.Proxycore

class ProxyService : Service() {

    private var wakeLock: PowerManager.WakeLock? = null
    private var wifiLock: WifiManager.WifiLock? = null

    companion object {
        const val CHANNEL_ID = "socks5_proxy_service_channel"
        const val NOTIFICATION_ID = 1001

        const val ACTION_START = "ACTION_START"
        const val ACTION_STOP = "ACTION_STOP"

        const val EXTRA_LOCAL_PORT = "EXTRA_LOCAL_PORT"
        const val EXTRA_SERVER_ADDR = "EXTRA_SERVER_ADDR"
        const val EXTRA_SERVER_PORT = "EXTRA_SERVER_PORT"
        const val EXTRA_TOKEN = "EXTRA_TOKEN"
        const val EXTRA_REMOTE_PORT = "EXTRA_REMOTE_PORT"
        const val EXTRA_SOCKS_USER = "EXTRA_SOCKS_USER"
        const val EXTRA_SOCKS_PASS = "EXTRA_SOCKS_PASS"

        var isServiceRunning = false
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
        acquireLocks()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val action = intent?.action ?: ACTION_START

        if (action == ACTION_STOP) {
            stopProxy()
            stopForeground(STOP_FOREGROUND_REMOVE)
            stopSelf()
            return START_NOT_STICKY
        }

        val localPort = intent?.getIntExtra(EXTRA_LOCAL_PORT, 10808) ?: 10808
        val serverAddr = intent?.getStringExtra(EXTRA_SERVER_ADDR) ?: ""
        val serverPort = intent?.getIntExtra(EXTRA_SERVER_PORT, 7000) ?: 7000
        val token = intent?.getStringExtra(EXTRA_TOKEN) ?: ""
        val remotePort = intent?.getIntExtra(EXTRA_REMOTE_PORT, 10001) ?: 10001
        val socksUser = intent?.getStringExtra(EXTRA_SOCKS_USER) ?: ""
        val socksPass = intent?.getStringExtra(EXTRA_SOCKS_PASS) ?: ""

        val notification = createNotification("Proxy running on VPS:$remotePort (Local:$localPort)")
        startForeground(NOTIFICATION_ID, notification)

        // Jalankan SOCKS5 + FRP di Go Core
        Thread {
            val result = Proxycore.startServer(
                localPort.toLong(),
                serverAddr,
                serverPort.toLong(),
                token,
                remotePort.toLong(),
                socksUser,
                socksPass
            )
            isServiceRunning = result == "OK"
        }.start()

        isServiceRunning = true
        return START_STICKY
    }

    private fun stopProxy() {
        Thread {
            Proxycore.stopServer()
        }.start()
        isServiceRunning = false
        releaseLocks()
    }

    private fun acquireLocks() {
        // High-Performance WakeLock (mencegah CPU sleep saat layar mati)
        val powerManager = getSystemService(Context.POWER_SERVICE) as PowerManager
        wakeLock = powerManager.newWakeLock(
            PowerManager.PARTIAL_WAKE_LOCK,
            "MobileSocks5Proxy::WakeLock"
        ).apply {
            setReferenceCounted(false)
            acquire(24 * 60 * 60 * 1000L) // 24 hours lock
        }

        // High-Performance WifiLock (mencegah WiFi masuk mode power-save latency tinggi)
        val wifiManager = applicationContext.getSystemService(Context.WIFI_SERVICE) as WifiManager
        wifiLock = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            wifiManager.createWifiLock(WifiManager.WIFI_MODE_FULL_LOW_LATENCY, "MobileSocks5Proxy::WifiLock")
        } else {
            wifiManager.createWifiLock(WifiManager.WIFI_MODE_FULL_HIGH_PERF, "MobileSocks5Proxy::WifiLock")
        }.apply {
            setReferenceCounted(false)
            acquire()
        }
    }

    private fun releaseLocks() {
        try {
            if (wakeLock?.isHeld == true) wakeLock?.release()
            if (wifiLock?.isHeld == true) wifiLock?.release()
        } catch (_: Exception) {}
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                "Mobile SOCKS5 Proxy Service",
                NotificationManager.IMPORTANCE_LOW
            ).apply {
                description = "Runs SOCKS5 & FRP Tunnel in background"
            }
            val manager = getSystemService(NotificationManager::class.java)
            manager.createNotificationChannel(channel)
        }
    }

    private fun createNotification(content: String): Notification {
        val intent = Intent(this, MainActivity::class.java)
        val pendingIntent = PendingIntent.getActivity(
            this, 0, intent,
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
        )

        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle("SOCKS5 + FRP Active")
            .setContentText(content)
            .setSmallIcon(android.R.drawable.stat_notify_sync)
            .setContentIntent(pendingIntent)
            .setOngoing(true)
            .build()
    }

    override fun onDestroy() {
        stopProxy()
        super.onDestroy()
    }
}
