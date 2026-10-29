using CommonFeedModels.Enums;
using MessagePack;

namespace Common.EventApi.Cache.Prague.Models;

[MessagePackObject]
public class SlimQuote
{
    public SlimQuote()
    {
        
    }

    public SlimQuote(string name,
        string? line,
        QuoteStatus status,
        QuoteStatus? statusManual,
        WinStatus winStatusId,
        WinStatus? winStatusIdManual,
        bool isDynamic,
        long timestamp,
        double quotesActual,
        double quotesFair,
        double? quotesManual,
        bool? isFeatured,
        bool? isBalancedLine,
        long? customerId)
    {
        Name = name;
        Line = line;
        Status = status;
        StatusManual = statusManual;
        WinStatusId = winStatusId;
        WinStatusIdManual = winStatusIdManual;
        IsDynamic = isDynamic;
        Timestamp = timestamp;
        QuotesActual = quotesActual;
        QuotesFair = quotesFair;
        QuotesManual = quotesManual;
        IsFeatured = isFeatured;
        IsBalancedLine = isBalancedLine;
        CustomerId = customerId;
    }

    [Key(0)]
    public string Name { get; init; }

    [Key(3)]
    public bool IsDynamic { get; init; }

    [Key(4)]
    public long Timestamp { get; init; }

    [Key(6)]
    public string? Line { get; init; }

    [Key(8)]
    public double QuotesActual { get; init; }

    [Key(9)]
    public double QuotesFair { get; init; }

    [Key(10)]
    public double? QuotesManual { get; init; }

    [Key(12)]
    public QuoteStatus Status { get; init; }

    [Key(13)]
    public QuoteStatus? StatusManual { get; init; }

    [Key(19)]
    public WinStatus WinStatusId { get; init; }

    [Key(20)]
    public WinStatus? WinStatusIdManual { get; init; }

    [Key(23)]
    public bool? IsFeatured { get; init; }

    [Key(26)]
    public bool? IsBalancedLine { get; init; }

    [Key(27)]
    public long? CustomerId { get; init; }
}
