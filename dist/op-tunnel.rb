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

    # SSH distribution files — ssh.config is installed here so it is
    # available via share/"op-tunnel" in post_install (buildpath is nil there)
    (share/"op-tunnel").install "dist/op-tunnel-sshd.conf", "dist/ssh.config"
    bin.install "dist/op-tunnel-setup"
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

    # Install SSH client config fragment
    op_tunnel_dir = Pathname.new("#{Dir.home}/.local/share/op-tunnel")
    op_tunnel_dir.mkpath
    cp share/"op-tunnel"/"ssh.config", op_tunnel_dir/"ssh.config"

    ohai "op-tunnel installed!"
    puts <<~EOS
      The SSH config fragment has been installed to:
        ~/.local/share/op-tunnel/ssh.config

      Add the following inside each Host block in ~/.ssh/config for
      hosts where you want op-tunnel active (requires OpenSSH 7.3+):

        Host myserver
            Include ~/.local/share/op-tunnel/ssh.config

      On each remote host, run once to configure sshd:
        sudo op-tunnel-setup
        (Skip on stock Debian/Ubuntu — AcceptEnv LANG LC_* already covers LC_OP_TUNNEL_SOCK)

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

      To activate tunneling for a remote host, add inside its Host block
      in ~/.ssh/config:
        Include ~/.local/share/op-tunnel/ssh.config

      On remote hosts (except stock Debian/Ubuntu), configure sshd once:
        sudo op-tunnel-setup
    EOS
  end

  test do
    assert_predicate bin/"op-tunnel-server", :executable?
    assert_predicate bin/"op-tunnel-client", :executable?
  end
end
