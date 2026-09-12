using MessagePack;

namespace Common.IntegratedEventApi.Models.Branded;

[MessagePackObject]
public class BaseProviderObject
{
    [Key(0)]
    public long Id { get; set; }
    [Key(1)]
    public virtual string Name { get; set; }
    [Key(2)]
    public string? ProviderSourceKey { get; set; }
    [Key(3)]
    public long ProviderKey { get; set; }
}
