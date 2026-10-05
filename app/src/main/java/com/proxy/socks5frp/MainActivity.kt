package com.proxy.socks5frp

import android.content.Context
import android.content.Intent
import android.graphics.Color
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.PowerManager
import android.provider.Settings
import android.widget.Button
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import com.google.android.material.textfield.TextInputEditText
import java.net.NetworkInterface
import java.util.Collections

class MainActivity : AppCompatActivity() {

    private lateinit var tvStatus: TextView
    private lateinit var tvDeviceIP: TextView
    private lateinit var btnToggle: Button
    private lateinit var btnBatteryOpt: Button

    private lateinit var etServerAddr: TextInputEditText
    private lateinit var etServerPort: TextInputEditText
    private lateinit var etToken: TextInputEditText
    private lateinit var etRemotePort: TextInputEditText
    private lateinit var etLocalPort: TextInputEditText
    private lateinit var etSocksUser: TextInputEditText
    private lateinit var etSocksPass: TextInputEditText

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        initViews()
        loadPreferences()
        updateUIState()
        refreshIP()

        btnToggle.setOnClickListener {
            if (ProxyService.isServiceRunning) {
                stopProxyService()
            } else {
                startProxyService()
            }
        }

        btnBatteryOpt.setOnClickListener {
            requestBatteryOptimizationExemption()
        }
    }

    private fun initViews() {
        tvStatus = findViewById(R.id.tvStatus)
        tvDeviceIP = findViewById(R.id.tvDeviceIP)
        btnToggle = findViewById(R.id.btnToggle)
        btnBatteryOpt = findViewById(R.id.btnBatteryOpt)

        etServerAddr = findViewById(R.id.etServerAddr)
        etServerPort = findViewById(R.id.etServerPort)
        etToken = findViewById(R.id.etToken)
        etRemotePort = findViewById(R.id.etRemotePort)
        etLocalPort = findViewById(R.id.etLocalPort)
        etSocksUser = findViewById(R.id.etSocksUser)
        etSocksPass = findViewById(R.id.etSocksPass)
    }

    private fun startProxyService() {
        savePreferences()

        val serviceIntent = Intent(this, ProxyService::class.java).apply {
            action = ProxyService.ACTION_START
            putExtra(ProxyService.EXTRA_SERVER_ADDR, etServerAddr.text.toString().trim())
            putExtra(ProxyService.EXTRA_SERVER_PORT, etServerPort.text.toString().trim().toIntOrNull() ?: 7000)
            putExtra(ProxyService.EXTRA_TOKEN, etToken.text.toString().trim())
            putExtra(ProxyService.EXTRA_REMOTE_PORT, etRemotePort.text.toString().trim().toIntOrNull() ?: 10001)
            putExtra(ProxyService.EXTRA_LOCAL_PORT, etLocalPort.text.toString().trim().toIntOrNull() ?: 10808)
            putExtra(ProxyService.EXTRA_SOCKS_USER, etSocksUser.text.toString().trim())
            putExtra(ProxyService.EXTRA_SOCKS_PASS, etSocksPass.text.toString().trim())
        }

        ContextCompat.startForegroundService(this, serviceIntent)
        ProxyService.isServiceRunning = true
        updateUIState()
        Toast.makeText(this, "Proxy Starting...", Toast.LENGTH_SHORT).show()
    }

    private fun stopProxyService() {
        val serviceIntent = Intent(this, ProxyService::class.java).apply {
            action = ProxyService.ACTION_STOP
        }
        startService(serviceIntent)
        ProxyService.isServiceRunning = false
        updateUIState()
        Toast.makeText(this, "Proxy Stopped", Toast.LENGTH_SHORT).show()
    }

    private fun updateUIState() {
        if (ProxyService.isServiceRunning) {
            tvStatus.text = "Status: RUNNING (Proxy + FRP Active)"
            tvStatus.setTextColor(Color.parseColor("#22C55E"))
            btnToggle.text = "STOP PROXY TUNNEL"
            btnToggle.backgroundTintList = ContextCompat.getColorStateList(this, android.R.color.holo_red_dark)
        } else {
            tvStatus.text = "Status: STOPPED"
            tvStatus.setTextColor(Color.parseColor("#EF4444"))
            btnToggle.text = "START PROXY TUNNEL"
            btnToggle.backgroundTintList = ContextCompat.getColorStateList(this, android.R.color.holo_green_dark)
        }
    }

    private fun refreshIP() {
        Thread {
            val ip = getLocalIpAddress()
            runOnUiThread {
                tvDeviceIP.text = "Local Device IP: $ip"
            }
        }.start()
    }

    private fun getLocalIpAddress(): String {
        try {
            val interfaces = Collections.list(NetworkInterface.getNetworkInterfaces())
            for (intf in interfaces) {
                val addrs = Collections.list(intf.inetAddresses)
                for (addr in addrs) {
                    if (!addr.isLoopbackAddress && addr.hostAddress?.indexOf(':') ?: -1 < 0) {
                        return addr.hostAddress ?: "Unknown"
                    }
                }
            }
        } catch (_: Exception) {}
        return "127.0.0.1"
    }

    private fun requestBatteryOptimizationExemption() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
            val powerManager = getSystemService(Context.POWER_SERVICE) as PowerManager
            if (!powerManager.isIgnoringBatteryOptimizations(packageName)) {
                val intent = Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS).apply {
                    data = Uri.parse("package:$packageName")
                }
                startActivity(intent)
            } else {
                Toast.makeText(this, "Battery optimization already disabled!", Toast.LENGTH_SHORT).show()
            }
        }
    }

    private fun savePreferences() {
        val prefs = getSharedPreferences("proxy_settings", Context.MODE_PRIVATE)
        prefs.edit().apply {
            putString("server_addr", etServerAddr.text.toString())
            putString("server_port", etServerPort.text.toString())
            putString("token", etToken.text.toString())
            putString("remote_port", etRemotePort.text.toString())
            putString("local_port", etLocalPort.text.toString())
            putString("socks_user", etSocksUser.text.toString())
            putString("socks_pass", etSocksPass.text.toString())
            apply()
        }
    }

    private fun loadPreferences() {
        val prefs = getSharedPreferences("proxy_settings", Context.MODE_PRIVATE)
        etServerAddr.setText(prefs.getString("server_addr", ""))
        etServerPort.setText(prefs.getString("server_port", "7000"))
        etToken.setText(prefs.getString("token", ""))
        etRemotePort.setText(prefs.getString("remote_port", "10001"))
        etLocalPort.setText(prefs.getString("local_port", "10808"))
        etSocksUser.setText(prefs.getString("socks_user", ""))
        etSocksPass.setText(prefs.getString("socks_pass", ""))
    }
}
