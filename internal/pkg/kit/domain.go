package kit

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"github.com/likexian/whois"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"net"
	"time"
)

const (
	whiosMaxRetries = 3
)

func (m *Manager) DomainKits() []server.ServerTool {
	domainCheck := server.ServerTool{
		Tool: mcp.NewTool("domain_base_info_lookup",
			mcp.WithDescription("domain base info query 域名基本信息查询"),
			mcp.WithString("domain",
				mcp.Required(),
				mcp.Description("the domain to check 要查询的域名"),
			),
		),
		Handler: m.domainCheckHandler,
	}
	return []server.ServerTool{domainCheck}
}

func (m *Manager) domainCheckHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if domain, ok := request.Params.Arguments["domain"].(string); ok {
		baseInfo, err := getDomainBaseInfo(domain)
		if err != nil {
			return mcp.NewToolResultText(operationFailed), err
		}
		return mcp.NewToolResultText(j(baseInfo)), nil
	}
	return mcp.NewToolResultText(operationFailed), unSupportOperation
}

type domainBaseInfo struct {
	DNSNameServers []string            `json:"dns_nameservers,omitempty"`
	IpAddresses    []string            `json:"ip_addresses,omitempty"`
	MxRecords      []*net.MX           `json:"mx_records,omitempty"`
	WhoisInfo      string              `json:"whois_info,omitempty"`
	Certificates   []*x509.Certificate `json:"certificate,omitempty"`
}

func getDomainBaseInfo(domain string) (*domainBaseInfo, error) {
	baseInfo := &domainBaseInfo{}
	// 1. Check DNS NS records
	nsRecords, err := net.LookupNS(domain)

	if err == nil && len(nsRecords) > 0 {
		baseInfo.DNSNameServers = make([]string, 0, len(nsRecords))
		for _, nsRecord := range nsRecords {
			baseInfo.DNSNameServers = append(baseInfo.DNSNameServers, nsRecord.Host)
		}
	}

	// 2. Check DNS A records
	ipRecords, err := net.LookupIP(domain)
	if err == nil && len(ipRecords) > 0 {
		baseInfo.IpAddresses = make([]string, 0, len(ipRecords))
		for _, ipRecord := range ipRecords {
			baseInfo.IpAddresses = append(baseInfo.IpAddresses, ipRecord.String())
		}
	}

	// 3. Check DNS MX records
	mxRecords, err := net.LookupMX(domain)
	if err == nil && len(mxRecords) > 0 {
		baseInfo.MxRecords = make([]*net.MX, 0, len(mxRecords))
		for _, mxRecord := range mxRecords {
			baseInfo.MxRecords = append(baseInfo.MxRecords, mxRecord)
		}
	}

	// 4. Check WHOIS information with retry
	var whoisResult string

	for i := 0; i < whiosMaxRetries; i++ {
		result, err := whois.Whois(domain)
		if err == nil {
			whoisResult = result
			break
		}
		if i < whiosMaxRetries-1 {
			time.Sleep(time.Second * 2) // Wait 2 seconds before retry
		}
	}

	if whoisResult != "" {
		baseInfo.WhoisInfo = whoisResult
	}
	// 5. Check SSL certificate with timeout
	conn, err := tls.DialWithDialer(&net.Dialer{
		Timeout: 5 * time.Second,
	}, "tcp", domain+":443", &tls.Config{
		InsecureSkipVerify: true,
	})
	if err == nil {
		defer conn.Close()
		state := conn.ConnectionState()
		if len(state.PeerCertificates) > 0 {
			baseInfo.Certificates = state.PeerCertificates
		}
	}
	return baseInfo, nil
}
