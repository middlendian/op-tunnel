class OpTunnel < Formula
  desc "Tunnel 1Password CLI (op) commands over SSH"
  homepage "https://github.com/middlendian/op-tunnel"
  url "https://github.com/middlendian/op-tunnel/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "PLACEHOLDER"
  license "MIT"

  depends_on "go" => :build

  def install
    system "make", "build"
    bin.install "bin/op-tunnel-server"
    bin.install "bin/op-tunnel-client"

    # Service files
    prefix.install "dist/com.middlendian.op-tunnel-server.plist"
    prefix.install "dist/op-tunnel-server.service"
  end

  def post_install
    # Symlink client as `op` — overrides native op binary
    bin.install_symlink bin/"op-tunnel-client" => "op"

    # Install LaunchAgent with correct paths
    plist = prefix/"com.middlendian.op-tunnel-server.plist"
    inreplace plist, "HOMEBREW_PREFIX", HOMEBREW_PREFIX
    launch_agent_dir = Pathname.new("#{Dir.home}/Library/LaunchAgents")
    launch_agent_dir.mkpath
    ln_sf plist, launch_agent_dir/"com.middlendian.op-tunnel-server.plist"

    ohai "op-tunnel installed!"
    puts <<~EOS
      Add to your ~/.ssh/config:

        Host *
            RemoteForward ~/.local/share/op-tunnel/client/op-tunnel.sock ~/.local/share/op-tunnel/server/op-tunnel.sock
            SetEnv LC_OP_TUNNEL_SOCK=~/.local/share/op-tunnel/client/op-tunnel.sock
            StreamLocalBindUnlink yes
            ServerAliveInterval 30
            ServerAliveCountMax 6

      The server LaunchAgent has been installed and will start on next login.
      To start it now:
        launchctl load ~/Library/LaunchAgents/com.middlendian.op-tunnel-server.plist
    EOS
  end

  def caveats
    <<~EOS
      op-tunnel-client has been symlinked as `op`.
      When LC_OP_TUNNEL_SOCK is set (via SSH), op commands are tunneled.
      Otherwise, the real op binary is called directly.

      Ensure the remote sshd has: AcceptEnv LC_*
    EOS
  end

  test do
    assert_predicate bin/"op-tunnel-server", :executable?
    assert_predicate bin/"op-tunnel-client", :executable?
  end
end
