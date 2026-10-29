using Prague.Feed.Cache.Core;
using CommonFeedModels.Enums;
using MessagePack;

namespace Common.EventApi.Cache.Prague.Models;

[MessagePackObject]
[DataCache(CacheClassName = nameof(SlimTicketTypeCache))]
[DataCacheTopic("Cache.[v:brandId].TicketType")]
public sealed partial class SlimTicketType
{
    public SlimTicketType()
    {
        
    }

    public SlimTicketType(long ticketTypeId,
        long eventId,
        string name,
        long timestamp,
        bool isLive,
        bool isListable,
        TicketTypeStatus status,
        TicketTypeStatus? statusManual,
        decimal? limitManualSingle,
        decimal? limitManualBundle,
        decimal? limitManualPurchaseBuilder)
    {
        TicketTypeId = ticketTypeId;
        EventId = eventId;
        Name = name;
        Timestamp = timestamp;
        IsLive = isLive;
        IsListable = isListable;
        Status = status;
        StatusManual = statusManual;
        LimitManualSingle = limitManualSingle;
        LimitManualBundle = limitManualBundle;
        LimitManualPurchaseBuilder = limitManualPurchaseBuilder;
    }

    [Key(0)]
    [DataCacheKey]
    public (long EventId, long TicketTypeId) Id => (EventId, TicketTypeId);

    [Key(1)]
    [DataCacheIndex]
    public long EventId { get; set; }

    [Key(2)]
    [DataCacheIndex]
    public long TicketTypeId { get; set; }

    [Key(3)]
    public string Name { get; set; } = string.Empty;

    [Key(6)]
    [DataCacheIndex]
    public TicketTypeStatus Status { get; set; }

    [Key(7)]
    public TicketTypeStatus? StatusManual { get; set; }

    [Key(8)]
    public TicketTypeStatus? StatusWatchDog { get; set; }

    [Key(9)]
    [DataCacheIndex]
    public bool IsLive { get; set; }

    [Key(10)]
    public bool IsListable { get; set; }

    [Key(21)]
    public decimal? LimitManualSingle { get; set; }

    [Key(22)]
    public decimal? LimitManualBundle { get; set; }

    [Key(23)]
    public decimal? LimitManualPurchaseBuilder { get; set; }

    [Key(24)]
    [DataCacheIndex(DataCacheIndexType.Range)]
    [DataCacheFromTimestamp]
    public long Timestamp { get; set; }

    [Key(25)]
    public SlimQuote[] Quotes { get; set; } = [];
}
